// Copyright 2022 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/mitchellh/copystructure"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes      = uint64(1000000000)   // 1 GB
	MinimumCVSVolumeSizeBytesSW = uint64(322122547200) // 300 GiB
	MinimumCVSVolumeSizeBytesHW = uint64(107374182400) // 100 GiB
	MinimumAPIVersion           = "1.1.6"
	MinimumSDEVersion           = "2020.10.0"

	defaultServiceLevel    = api.UserServiceLevel1
	defaultNfsMountOptions = "-o nfsvers=3"
	defaultSecurityStyle   = "unix"
	defaultSnapshotDir     = "false"
	defaultSnapshotReserve = ""
	defaultUnixPermissions = "0777"
	defaultStorageClass    = api.StorageClassHardware
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"
	defaultNetwork         = "default"

	// Constants for internal pool attributes
	Size            = "size"
	ServiceLevel    = "serviceLevel"
	SnapshotDir     = "snapshotDir"
	SnapshotReserve = "snapshotReserve"
	ExportRule      = "exportRule"
	Network         = "network"
	Region          = "region"
	Zone            = "zone"
	StorageClass    = "storageClass"
	UnixPermissions = "unixPermissions"

	// Topology label names
	topologyZoneLabel   = drivers.TopologyLabelPrefix + "/" + Zone
	topologyRegionLabel = drivers.TopologyLabelPrefix + "/" + Region
)

// NFSStorageDriver is for storage provisioning using Cloud Volumes Service in GCP
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.GCPNFSStorageDriverConfig
	API                 api.GCPClient
	telemetry           *Telemetry
	apiVersion          *utils.Version
	sdeVersion          *utils.Version
	tokenRegexp         *regexp.Regexp
	csiRegexp           *regexp.Regexp
	apiRegions          []string
	pools               map[string]storage.Pool
	volumeCreateTimeout time.Duration
}

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}

func (d *NFSStorageDriver) GetConfig() *drivers.GCPNFSStorageDriverConfig {
	return &d.Config
}

func (d *NFSStorageDriver) GetAPI() api.GCPClient {
	return d.API
}

func (d *NFSStorageDriver) getTelemetry() *Telemetry {
	return d.telemetry
}

// Name returns the name of this driver
func (d *NFSStorageDriver) Name() string {
	return drivers.GCPNFSStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance
func (d *NFSStorageDriver) defaultBackendName() string {
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), d.Config.APIKey.PrivateKeyID[0:5])
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

	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// validateName checks that the name of a new volume matches the requirements of a creation token
func (d *NFSStorageDriver) validateName(name string) error {
	if !d.tokenRegexp.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be at most 80 characters long, "+
			"begin with a character, and contain only characters, digits, and hyphens", name)
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

func (d *NFSStorageDriver) makeNetworkPath(network string) string {

	projectNumber := d.Config.ProjectNumber
	if d.Config.HostProjectNumber != "" {
		projectNumber = d.Config.HostProjectNumber
	}
	return fmt.Sprintf("projects/%s/global/networks/%s", projectNumber, network)
}

// applyMinimumVolumeSizeSW applies the volume size rules for CVS-SO
func (d *NFSStorageDriver) applyMinimumVolumeSizeSW(sizeBytes uint64) uint64 {

	if sizeBytes < MinimumCVSVolumeSizeBytesSW {
		return MinimumCVSVolumeSizeBytesSW
	}

	return sizeBytes
}

// applyMinimumVolumeSizeHW applies the volume size rules for CVS-PO
func (d *NFSStorageDriver) applyMinimumVolumeSizeHW(sizeBytes uint64) uint64 {

	if sizeBytes < MinimumCVSVolumeSizeBytesHW {
		return MinimumCVSVolumeSizeBytesHW
	}

	return sizeBytes
}

// Initialize initializes this driver from the provided config
func (d *NFSStorageDriver) Initialize(
	ctx context.Context, context tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	commonConfig.DriverContext = context
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	d.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// Parse the config
	config, err := d.initializeGCPConfig(ctx, configJSON, commonConfig, backendSecret)
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

	if d.API, err = d.initializeGCPAPIClient(ctx, &d.Config); err != nil {
		return fmt.Errorf("error initializing %s API client. %v", d.Name(), err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
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
	ctx context.Context, config *drivers.GCPNFSStorageDriverConfig,
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

	if config.StorageClass == "" {
		config.StorageClass = defaultStorageClass
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

	if config.UnixPermissions == "" {
		config.UnixPermissions = defaultUnixPermissions
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	if config.ExportRule == "" {
		config.ExportRule = defaultExportRule
	}

	if config.Network == "" {
		config.Network = defaultNetwork
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
		"Network":                    config.Network,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	}).Debugf("Configuration defaults")

	return nil
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NFSStorageDriver) initializeStoragePools(ctx context.Context) error {

	d.pools = make(map[string]storage.Pool)

	if len(d.Config.Storage) == 0 {

		Logc(ctx).Debug("No vpools defined, reporting single pool.")

		// No vpools defined, so report region/zone as a single pool
		pool := storage.NewStoragePool(nil, d.poolName("pool"))

		pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
		pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
		pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)
		if d.Config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes()[Size] = d.Config.Size
		pool.InternalAttributes()[ServiceLevel] = d.Config.ServiceLevel
		pool.InternalAttributes()[StorageClass] = d.Config.StorageClass
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes()[UnixPermissions] = d.Config.UnixPermissions
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[Network] = d.Config.Network
		pool.InternalAttributes()[Region] = d.Config.Region
		pool.InternalAttributes()[Zone] = d.Config.Zone

		pool.SetSupportedTopologies(d.Config.SupportedTopologies)

		d.pools[pool.Name()] = pool

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

			storageClass := d.Config.StorageClass
			if vpool.StorageClass != "" {
				storageClass = vpool.StorageClass
			}

			snapshotDir := d.Config.SnapshotDir
			if vpool.SnapshotDir != "" {
				snapshotDir = vpool.SnapshotDir
			}

			snapshotReserve := d.Config.SnapshotReserve
			if vpool.SnapshotReserve != "" {
				snapshotReserve = vpool.SnapshotReserve
			}
			unixPermissions := d.Config.UnixPermissions
			if vpool.UnixPermissions != "" {
				unixPermissions = vpool.UnixPermissions
			}

			exportRule := d.Config.ExportRule
			if vpool.ExportRule != "" {
				exportRule = vpool.ExportRule
			}

			network := d.Config.Network
			if vpool.Network != "" {
				network = vpool.Network
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[ServiceLevel] = serviceLevel
			pool.InternalAttributes()[StorageClass] = storageClass
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[Network] = network
			pool.InternalAttributes()[Region] = region
			pool.InternalAttributes()[Zone] = zone

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}

	return nil
}

// initializeGCPConfig parses the GCP config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeGCPConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.GCPNFSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeGCPConfig", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeGCPConfig")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeGCPConfig")
	}

	config := &drivers.GCPNFSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into GCPNFSStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		err = config.InjectSecrets(backendSecret)
		if err != nil {
			return nil, fmt.Errorf("could not inject backend secret; err: %v", err)
		}
	}

	return config, nil
}

// initializeGCPAPIClient returns an GCP API client.
func (d *NFSStorageDriver) initializeGCPAPIClient(
	ctx context.Context, config *drivers.GCPNFSStorageDriverConfig,
) (api.GCPClient, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeGCPAPIClient", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeGCPAPIClient")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeGCPAPIClient")
	}

	client := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		APIURL:          config.APIURL,
		APIAudienceURL:  config.APIAudienceURL,
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

	// Ensure API region is set
	if d.Config.APIRegion == "" {
		return errors.New("apiRegion in config must be specified")
	}

	// Validate API version
	if d.apiVersion, d.sdeVersion, err = d.API.GetVersion(ctx); err != nil {
		return err
	}

	// Log the service level mapping
	_, _ = d.API.GetServiceLevels(ctx)

	if _, err := d.API.GetVolumes(ctx); err != nil {
		return fmt.Errorf("could not read volumes in %s region; %v", d.Config.APIRegion, err)
	}

	if d.apiVersion.LessThan(utils.MustParseSemantic(MinimumAPIVersion)) {
		return fmt.Errorf("API version is %s, at least %s is required", d.apiVersion.String(), MinimumAPIVersion)
	}
	if d.sdeVersion.LessThan(utils.MustParseSemantic(MinimumSDEVersion)) {
		return fmt.Errorf("SDE version is %s, at least %s is required", d.sdeVersion.String(), MinimumSDEVersion)
	}

	Logc(ctx).WithField("region", d.Config.APIRegion).Debug("REST API access OK.")

	// Validate driver-level attributes

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level
		if !api.IsValidUserServiceLevel(pool.InternalAttributes()[ServiceLevel]) {
			return fmt.Errorf("invalid service level in pool %s: %s", poolName, pool.InternalAttributes()[ServiceLevel])
		}

		// Validate export rules
		for _, rule := range strings.Split(pool.InternalAttributes()[ExportRule], ",") {
			ipAddr := net.ParseIP(rule)
			_, netAddr, _ := net.ParseCIDR(rule)
			if ipAddr == nil && netAddr == nil {
				return fmt.Errorf("invalid address/CIDR for exportRule in pool %s: %s", poolName, rule)
			}
		}

		// Validate snapshot dir
		if pool.InternalAttributes()[SnapshotDir] != "" {
			_, err := strconv.ParseBool(pool.InternalAttributes()[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s: %v", poolName, err)
			}
		}

		// Validate snapshot reserve
		if pool.InternalAttributes()[SnapshotReserve] != "" {
			snapshotReserve, err := strconv.ParseInt(pool.InternalAttributes()[SnapshotReserve], 10, 0)
			if err != nil {
				return fmt.Errorf("invalid value for snapshotReserve in pool %s: %v", poolName, err)
			}
			if snapshotReserve < 0 || snapshotReserve > 90 {
				return fmt.Errorf("invalid value for snapshotReserve in pool %s: %s",
					poolName, pool.InternalAttributes()[SnapshotReserve])
			}
		}

		// Validate default size
		if _, err = utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
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
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
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
	pool, ok := d.pools[storagePool.Name()]
	if !ok {
		return fmt.Errorf("pool %s does not exist", storagePool.Name())
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

	// Take storage class from volume config (named CVSStorageClass to avoid name collision)
	// first (handles Docker case), then from pool.
	storageClass := volConfig.CVSStorageClass
	if storageClass == "" {
		storageClass = pool.InternalAttributes()[StorageClass]
		volConfig.CVSStorageClass = storageClass
	}

	unixPermissions := volConfig.UnixPermissions
	if unixPermissions == "" {
		unixPermissions = pool.InternalAttributes()[UnixPermissions]
		volConfig.UnixPermissions = unixPermissions
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
	if requestedSizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size])
		requestedSizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if checkMinVolumeSizeError := drivers.CheckMinVolumeSize(requestedSizeBytes,
		MinimumVolumeSizeBytes); checkMinVolumeSizeError != nil {
		return checkMinVolumeSizeError
	}

	// Apply volume size rules
	var sizeBytes uint64
	switch storageClass {
	case api.StorageClassSoftware:
		sizeBytes = d.applyMinimumVolumeSizeSW(requestedSizeBytes)
	case api.StorageClassHardware:
		sizeBytes = d.applyMinimumVolumeSizeHW(requestedSizeBytes)
	default:
		return fmt.Errorf("invalid storageClass: %s", storageClass)
	}

	if requestedSizeBytes < sizeBytes {

		Logc(ctx).WithFields(log.Fields{
			"name":          name,
			"requestedSize": requestedSizeBytes,
			"minimumSize":   sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable.")

		volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	userServiceLevel := volConfig.ServiceLevel
	if userServiceLevel == "" {
		userServiceLevel = pool.InternalAttributes()[ServiceLevel]
		volConfig.ServiceLevel = userServiceLevel
	}
	switch userServiceLevel {
	case api.UserServiceLevel1, api.UserServiceLevel2, api.UserServiceLevel3:
		break
	default:
		return fmt.Errorf("invalid service level: %s", userServiceLevel)
	}
	apiServiceLevel := api.GCPAPIServiceLevelFromUserServiceLevel(userServiceLevel)

	// Take snapshot directory from volume config first (handles Docker case), then from pool
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes()[SnapshotDir]
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir: %v", err)
	}

	// Take snapshot reserve from volume config first (handles Docker case), then from pool
	snapshotReserve := volConfig.SnapshotReserve
	if snapshotReserve == "" {
		snapshotReserve = pool.InternalAttributes()[SnapshotReserve]
	}
	var snapshotReservePtr *int64
	if snapshotReserve != "" {
		snapshotReserveInt, err := strconv.ParseInt(snapshotReserve, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for snapshotReserve: %v", err)
		}
		snapshotReservePtr = &snapshotReserveInt
		volConfig.SnapshotReserve = snapshotReserve
	}

	// Take network from volume config first (handles Docker case), then from pool
	network := volConfig.Network
	if network == "" {
		network = pool.InternalAttributes()[Network]
		volConfig.Network = network
	}
	gcpNetwork := d.makeNetworkPath(network)

	// TODO: Remove when software storage class allows NFSv4
	protocolTypes := []string{api.ProtocolTypeNFSv3}
	switch storageClass {
	case api.StorageClassSoftware:
		break
	case api.StorageClassHardware:
		protocolTypes = append(protocolTypes, api.ProtocolTypeNFSv4)
	default:
		return fmt.Errorf("invalid storageClass: %s", storageClass)
	}

	// Take the zone from volume config first (handles Docker case), then from pool
	zone := volConfig.Zone
	if zone == "" {
		zone = pool.InternalAttributes()[Zone]
	}

	zone = d.ensureTopologyRegionAndZone(ctx, volConfig, storageClass, zone)

	if storageClass == api.StorageClassSoftware && zone == "" {
		return fmt.Errorf("software volumes require zone")
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":    name,
		"size":             sizeBytes,
		"network":          network,
		"userServiceLevel": userServiceLevel,
		"apiServiceLevel":  apiServiceLevel,
		"storageClass":     storageClass,
		"snapshotDir":      snapshotDirBool,
		"snapshotReserve":  snapshotReserve,
		"protocolTypes":    protocolTypes,
		"zone":             zone,
	}).Debug("Creating volume.")

	apiExportRule := api.ExportRule{
		AllowedClients: pool.InternalAttributes()[ExportRule],
		Access:         api.AccessReadWrite,
		NFSv3:          api.Checked{Checked: true},
		NFSv4:          api.Checked{Checked: true},
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

	createRequest := &api.VolumeCreateRequest{
		Name:              volConfig.Name,
		Region:            d.Config.APIRegion,
		CreationToken:     name,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProtocolTypes:     protocolTypes,
		QuotaInBytes:      int64(sizeBytes),
		SecurityStyle:     defaultSecurityStyle,
		ServiceLevel:      apiServiceLevel,
		StorageClass:      storageClass,
		SnapshotDirectory: snapshotDirBool,
		SnapshotPolicy:    snapshotPolicy,
		SnapReserve:       snapshotReservePtr,
		UnixPermissions:   volConfig.UnixPermissions,
		Network:           gcpNetwork,
		Zone:              zone,
	}

	// Create the volume
	if err := d.API.CreateVolume(ctx, createRequest); err != nil {
		return err
	}

	// Get the volume
	newVolume, err := d.API.GetVolumeByCreationToken(ctx, name)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, newVolume, name)
}

func (d *NFSStorageDriver) ensureTopologyRegionAndZone(
	ctx context.Context, volConfig *storage.VolumeConfig, storageClass, configuredZone string,
) string {
	// Check if configured topology (zone/region) matches provided topology for software volumes
	// If zone is not configured, use the preferred zone from the topology
	if len(volConfig.PreferredTopologies) > 0 {
		if r, ok := volConfig.PreferredTopologies[0][topologyRegionLabel]; ok {
			if d.Config.APIRegion != r {
				Logc(ctx).WithFields(log.Fields{
					"configuredRegion": d.Config.APIRegion,
					"topologyRegion":   r,
				}).Warn("configured region does not match topology region")
			}
		}
		if storageClass == api.StorageClassSoftware {
			if z, ok := volConfig.PreferredTopologies[0][topologyZoneLabel]; ok {
				if configuredZone != "" && configuredZone != z {
					Logc(ctx).WithFields(log.Fields{
						"configuredZone": configuredZone,
						"topologyZone":   z,
					}).Warn("configured zone does not match topology zone")
				}
				if configuredZone == "" {
					configuredZone = z
				}
			}
		}
	}
	return configuredZone
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NFSStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {

	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshot

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
	// if snapshot specified, read snapshot / backup from source, fail if nonexistent
	// if no snap specified for hardware, create one, fail if error
	// if no snap specified for software, fail
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
		switch extantVolume.LifeCycleState {

		case api.StateCreating, api.StateRestoring:
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(fmt.Sprintf("volume state is %s, not %s",
				extantVolume.LifeCycleState, api.StateAvailable))

		case api.StateAvailable, api.StateUpdating:
			// Creation is done, so return VolumeExistsError to trigger the idempotency in Backend
			return drivers.NewVolumeExistsError(name)

		default:
			// Any unexpected state should be returned as an actual error.
			return fmt.Errorf("volume state is %s, not %s", extantVolume.LifeCycleState, api.StateAvailable)
		}
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
	sourceVolume, err = d.API.GetVolumeByID(ctx, sourceVolume.VolumeID)
	if err != nil {
		return fmt.Errorf("could not get source volume details: %v", err)
	}

	// Only one of the following will return a name & ID, the other is a no-op that returns empty strings,
	// so both values may be passed to the clone API.
	sourceSnapshotName, sourceSnapshotID, err := d.getSnapshotForClone(ctx, sourceVolume, snapshot)
	if err != nil {
		return err
	}
	sourceBackupName, sourceBackupID, err := d.getBackupForClone(ctx, sourceVolume, snapshot)
	if err != nil {
		return err
	}

	// Ensure we got exactly one clone source
	if sourceSnapshotID == "" && sourceBackupID == "" {
		return fmt.Errorf("could not determine source details")
	} else if sourceSnapshotID != "" && sourceBackupID != "" {
		return fmt.Errorf("cannot clone a volume from both snapshot and backup")
	}

	network := cloneVolConfig.Network
	if network == "" {
		Logc(ctx).Debugf("Network not found in volume config, using '%s'.", d.Config.Network)
		network = d.Config.Network
	}
	gcpNetwork := d.makeNetworkPath(network)

	if sourceVolume.StorageClass == api.StorageClassSoftware && sourceVolume.Zone == "" {
		return fmt.Errorf("software volumes require zone")
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshotName,
		"sourceBackup":   sourceBackupName,
	}).Debug("Cloning volume.")

	var labels []string
	labels = d.updateTelemetryLabels(ctx, sourceVolume)

	if storagePool == nil || storagePool.Name() == drivers.UnsetPool {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
		})
		poolLabels, err := storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
		if err != nil {
			return err
		}

		labels = storage.UpdateProvisioningLabels(poolLabels, labels)
	}

	createRequest := &api.VolumeCreateRequest{
		Name:              cloneVolConfig.Name,
		Region:            sourceVolume.Region,
		Zone:              sourceVolume.Zone,
		CreationToken:     name,
		ExportPolicy:      sourceVolume.ExportPolicy,
		Labels:            labels,
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		QuotaInBytes:      sourceVolume.QuotaInBytes,
		SecurityStyle:     defaultSecurityStyle,
		ServiceLevel:      sourceVolume.ServiceLevel,
		StorageClass:      sourceVolume.StorageClass,
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SnapshotPolicy:    sourceVolume.SnapshotPolicy,
		SnapReserve:       &sourceVolume.SnapReserve,
		SnapshotID:        sourceSnapshotID,
		UnixPermissions:   sourceVolume.UnixPermissions,
		BackupID:          sourceBackupID,
		Network:           gcpNetwork,
	}

	// Clone the volume
	if err := d.API.CreateVolume(ctx, createRequest); err != nil {
		return err
	}

	// Get the volume
	clone, err := d.API.GetVolumeByCreationToken(ctx, name)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, clone, name)
}

func (d *NFSStorageDriver) getSnapshotForClone(
	ctx context.Context, sourceVolume *api.Volume, snapshot string,
) (string, string, error) {

	if sourceVolume.StorageClass == api.StorageClassSoftware {
		return "", "", nil
	}

	var sourceSnapshot *api.Snapshot
	var err error

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.API.GetSnapshotForVolume(ctx, sourceVolume, snapshot)
		if err != nil {
			return "", "", fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.LifeCycleState != api.StateAvailable {
			return "", "", fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.LifeCycleState, api.StateAvailable)
		}

	} else {

		newSnapName := time.Now().UTC().Format(storage.SnapshotNameFormat)

		// No source snapshot specified, so create one
		snapshotCreateRequest := &api.SnapshotCreateRequest{
			VolumeID: sourceVolume.VolumeID,
			Name:     newSnapName,
		}

		if err = d.API.CreateSnapshot(ctx, snapshotCreateRequest); err != nil {
			return "", "", fmt.Errorf("could not create source snapshot: %v", err)
		}

		sourceSnapshot, err = d.API.GetSnapshotForVolume(ctx, sourceVolume, newSnapName)
		if err != nil {
			return "", "", fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.API.WaitForSnapshotState(
			ctx, sourceSnapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return "", "", err
		}
	}

	return sourceSnapshot.Name, sourceSnapshot.SnapshotID, nil
}

func (d *NFSStorageDriver) getBackupForClone(
	ctx context.Context, sourceVolume *api.Volume, snapshot string,
) (string, string, error) {

	if sourceVolume.StorageClass == api.StorageClassHardware {
		return "", "", nil
	}

	if snapshot == "" {
		return "", "", fmt.Errorf("source snapshot must be specified for a software volume")
	}

	var sourceBackup *api.Backup
	var err error

	// Get the source backup
	sourceBackup, err = d.API.GetBackupForVolume(ctx, sourceVolume, snapshot)
	if err != nil {
		return "", "", fmt.Errorf("could not find source backup: %v", err)
	}

	// Ensure backup is in a usable state
	if sourceBackup.LifeCycleState != api.StateAvailable {
		return "", "", fmt.Errorf(
			"source backup state is '%s', it must be '%s'", sourceBackup.LifeCycleState, api.StateAvailable,
		)
	}

	return sourceBackup.Name, sourceBackup.BackupID, nil
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
		// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
		unixPerms := volConfig.UnixPermissions
		if unixPerms == "" {
			unixPerms = d.Config.UnixPermissions
		}
		labels := d.updateTelemetryLabels(ctx, volume)
		labels = storage.DeleteProvisioningLabels(labels)

		if _, err := d.API.RelabelVolume(ctx, volume, labels); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, relabel failed: %v", err)
			return fmt.Errorf("could not import volume %s, relabel failed: %v", originalName, err)
		}
		_, err := d.API.WaitForVolumeStates(
			ctx, volume, []string{api.StateAvailable}, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return fmt.Errorf("could not import volume %s: %v", originalName, err)
		}
		if _, err = d.API.ChangeVolumeUnixPermissions(ctx, volume, unixPerms); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import volume, modifying unix permissions failed: %v", err)
			return fmt.Errorf("could not import volume %s, modifying unix permissions failed: %v", originalName, err)
		}
		_, err = d.API.WaitForVolumeStates(
			ctx, volume, []string{api.StateAvailable}, []string{api.StateError}, d.defaultTimeout())
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

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) updateTelemetryLabels(ctx context.Context, volume *api.Volume) []string {

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
func (d *NFSStorageDriver) waitForVolumeCreate(ctx context.Context, volume *api.Volume, volumeName string) error {

	state, err := d.API.WaitForVolumeStates(
		ctx, volume, []string{api.StateAvailable}, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		if state == api.StateCreating || state == api.StateRestoring {
			Logc(ctx).WithFields(log.Fields{
				"volume": volumeName,
			}).Debugf("Volume is in %s state.", state)
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
				_, errDeleteWait := d.API.WaitForVolumeStates(
					ctx, volume, []string{api.StateDeleted}, []string{api.StateError}, d.defaultTimeout())
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
func (d *NFSStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {

	name := volConfig.InternalName

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
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.LifeCycleState == api.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.API.WaitForVolumeStates(
			ctx, extantVolume, []string{api.StateDeleted}, []string{api.StateError}, d.volumeCreateTimeout)
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
	_, err = d.API.WaitForVolumeStates(
		ctx, volume, []string{api.StateDeleted}, []string{api.StateError}, d.defaultTimeout())
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

	if len(volume.MountPoints) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsServerIP = (volume.MountPoints)[0].Server
	publishInfo.NfsPath = (volume.MountPoints)[0].Export
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NFSStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NFSStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {

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
		// The GCP volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil, nil
	}

	// Use snapshot or backup API depending on CVS storage class
	switch extantVolume.StorageClass {
	case api.StorageClassHardware:
		return d.getSnapshot(ctx, extantVolume, snapConfig)
	case api.StorageClassSoftware:
		return d.getBackup(ctx, extantVolume, snapConfig)
	default:
		return nil, fmt.Errorf("unknown CVS storage class (%s) for volume %s",
			extantVolume.StorageClass, extantVolume.Name)
	}
}

// getSnapshot returns a snapshot object from a hardware volume.
func (d *NFSStorageDriver) getSnapshot(
	ctx context.Context, volume *api.Volume, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	snapshots, err := d.API.GetSnapshotsForVolume(ctx, volume)
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
				SizeBytes: volume.QuotaInBytes,
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	return nil, nil
}

// getBackup returns the C2C backup object from a software volume as a snapshot object.
func (d *NFSStorageDriver) getBackup(
	ctx context.Context, volume *api.Volume, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	backups, err := d.API.GetBackupsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	for _, backup := range *backups {
		if backup.Name == internalSnapName {

			created := backup.Created.UTC().Format(storage.SnapshotTimestampFormat)

			Logc(ctx).WithFields(log.Fields{
				"snapshotName": internalSnapName,
				"volumeName":   internalVolName,
				"created":      created,
			}).Debug("Found backup.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   created,
				SizeBytes: volume.QuotaInBytes,
				State:     d.getSnapshotStateForBackup(&backup),
			}, nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"backupName": internalSnapName,
		"volumeName": internalVolName,
	}).Warning("Backup not found.")

	return nil, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NFSStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {

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

	// Use snapshot or backup API depending on CVS storage class
	switch volume.StorageClass {
	case api.StorageClassHardware:
		return d.getSnapshots(ctx, volume, volConfig)
	case api.StorageClassSoftware:
		return d.getBackups(ctx, volume, volConfig)
	default:
		return nil, fmt.Errorf("unknown CVS storage class (%s) for volume %s", volume.StorageClass, volume.Name)
	}
}

// getSnapshots returns the snapshot objects from a hardware volume.
func (d *NFSStorageDriver) getSnapshots(
	ctx context.Context, volume *api.Volume, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {

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

// getBackups returns the C2C backup objects from a software volume as snapshot objects.
func (d *NFSStorageDriver) getBackups(
	ctx context.Context, volume *api.Volume, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {

	backups, err := d.API.GetBackupsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, backup := range *backups {

		// Filter out backups in unavailable states
		switch backup.LifeCycleState {
		case api.StateDisabled, api.StateDeleting, api.StateDeleted, api.StateError:
			continue
		}

		snapshotList = append(snapshotList, &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               backup.Name,
				InternalName:       backup.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   backup.Created.Format(storage.SnapshotTimestampFormat),
			SizeBytes: volume.QuotaInBytes,
			State:     d.getSnapshotStateForBackup(&backup),
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *NFSStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {

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
	sourceVolume, err = d.API.GetVolumeByID(ctx, sourceVolume.VolumeID)
	if err != nil {
		return nil, fmt.Errorf("could not get source volume details: %v", err)
	}

	// Use snapshot or backup API depending on CVS storage class
	switch sourceVolume.StorageClass {
	case api.StorageClassHardware:
		return d.createSnapshot(ctx, sourceVolume, snapConfig)
	case api.StorageClassSoftware:
		return d.createBackup(ctx, sourceVolume, snapConfig)
	default:
		return nil, fmt.Errorf("unknown CVS storage class (%s) for volume %s",
			sourceVolume.StorageClass, sourceVolume.Name)
	}
}

// createSnapshot creates a snapshot of a hardware volume.
func (d *NFSStorageDriver) createSnapshot(
	ctx context.Context, sourceVolume *api.Volume, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName

	// Construct a snapshot request
	snapshotCreateRequest := &api.SnapshotCreateRequest{
		VolumeID: sourceVolume.VolumeID,
		Name:     internalSnapName,
	}

	if err := d.API.CreateSnapshot(ctx, snapshotCreateRequest); err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	snapshot, err := d.API.GetSnapshotForVolume(ctx, sourceVolume, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not find snapshot: %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.API.WaitForSnapshotState(ctx, snapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Info("Snapshot created.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.Format(storage.SnapshotTimestampFormat),
		SizeBytes: sourceVolume.QuotaInBytes,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// createBackup creates a C2C backup of a software volume.
func (d *NFSStorageDriver) createBackup(
	ctx context.Context, sourceVolume *api.Volume, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName

	// Construct a backup request
	backupCreateRequest := &api.BackupCreateRequest{
		VolumeID: sourceVolume.VolumeID,
		Name:     internalSnapName,
	}

	if err := d.API.CreateBackup(ctx, backupCreateRequest); err != nil {
		return nil, fmt.Errorf("could not create backup: %v", err)
	}

	backup, err := d.API.GetBackupForVolume(ctx, sourceVolume, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not find backup: %v", err)
	}

	// Wait for backup creation to complete
	err = d.API.WaitForBackupStates(ctx, backup,
		[]string{api.StateCreating, api.StateAvailable}, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return nil, err
	}

	// Get backup once more to get latest completion percent, if any
	backup, err = d.API.GetBackupByID(ctx, backup.BackupID)
	if err != nil {
		return nil, fmt.Errorf("could not find backup: %v", err)
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   backup.Created.Format(storage.SnapshotTimestampFormat),
		SizeBytes: sourceVolume.QuotaInBytes,
		State:     d.getSnapshotStateForBackup(backup),
	}, nil
}

// getSnapshotStateForBackup examines a software volume C2C backup and returns the state as
// appropriate for a Trident snapshot object.
func (d *NFSStorageDriver) getSnapshotStateForBackup(backup *api.Backup) storage.SnapshotState {

	var state storage.SnapshotState

	switch backup.LifeCycleState {

	// return Online in an unexpected state so snapshot may still be deleted
	case api.StateDisabled, api.StateDeleting, api.StateDeleted, api.StateError:
		state = storage.SnapshotStateOnline

	// If the backup is Creating but reports some data transfer, promote its status to Uploading.
	case api.StateCreating:
		state = storage.SnapshotStateCreating
		if backup.ProgressPercentage > 0 {
			state = storage.SnapshotStateUploading
		}

	// If the backup is Available but reports incomplete data transfer, demote its status to Uploading.
	case api.StateAvailable, api.StateUpdating:
		state = storage.SnapshotStateOnline
		if backup.ProgressPercentage < 100 {
			state = storage.SnapshotStateUploading
		}
	}

	return state
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NFSStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {

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

	if volume.StorageClass == api.StorageClassSoftware {
		return errors.New("software volumes do not support snapshot restore")
	}

	snapshot, err := d.API.GetSnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	return d.API.RestoreSnapshot(ctx, volume, snapshot)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NFSStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {

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
		// The GCP volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil
	}

	// Use snapshot or backup API depending on CVS storage class
	switch extantVolume.StorageClass {
	case api.StorageClassHardware:
		return d.deleteSnapshot(ctx, extantVolume, snapConfig)
	case api.StorageClassSoftware:
		return d.deleteBackup(ctx, extantVolume, snapConfig)
	default:
		return fmt.Errorf("unknown CVS storage class (%s) for volume %s", extantVolume.StorageClass, extantVolume.Name)
	}
}

// deleteSnapshot deletes a snapshot of a hardware volume.
func (d *NFSStorageDriver) deleteSnapshot(
	ctx context.Context, volume *api.Volume, snapConfig *storage.SnapshotConfig,
) error {

	internalSnapName := snapConfig.InternalName

	snapshot, err := d.API.GetSnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	if err := d.API.DeleteSnapshot(ctx, volume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	return d.API.WaitForSnapshotState(ctx, snapshot, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
}

// deleteBackup deletes a C2C backup of a software volume.
func (d *NFSStorageDriver) deleteBackup(
	ctx context.Context, volume *api.Volume, snapConfig *storage.SnapshotConfig,
) error {

	internalSnapName := snapConfig.InternalName

	backup, err := d.API.GetBackupForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find backup %s: %v", internalSnapName, err)
	}

	if err := d.API.DeleteBackup(ctx, volume, backup); err != nil {
		return err
	}

	// Wait for backup deletion to complete
	return d.API.WaitForBackupStates(ctx, backup,
		[]string{api.StateDeleted}, []string{api.StateError}, d.defaultTimeout())
}

// List returns the list of volumes associated with this tenant
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

// Get tests for the existence of a volume
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
		return utils.UnsupportedCapacityRangeError(fmt.Errorf("requested size %d is less than existing volume size %d",
			sizeBytes,
			volume.QuotaInBytes))
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Resize the volume
	if _, err := d.API.ResizeVolume(ctx, volume, int64(sizeBytes)); err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
	}

	// Wait for resize operation to complete
	_, err = d.API.WaitForVolumeStates(
		ctx, volume, []string{api.StateAvailable}, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return err
}

// GetStorageBackendSpecs retrieves storage capabilities and register pools with specified backend.
func (d *NFSStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {

	backend.SetName(d.BackendName())

	for _, pool := range d.pools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	return nil
}

func (d *NFSStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
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

	if len(volume.MountPoints) == 0 {
		return fmt.Errorf("volume %s has no mount points", volConfig.InternalName)
	}

	// Just use the first mount target found
	volConfig.AccessInfo.NfsServerIP = (volume.MountPoints)[0].Server
	volConfig.AccessInfo.NfsPath = (volume.MountPoints)[0].Export
	volConfig.FileSystem = "nfs"

	return nil
}

func (d *NFSStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NFSStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.GCPConfig = &d.Config
}

func (d *NFSStorageDriver) GetExternalConfig(ctx context.Context) interface{} {

	// Clone the config so we don't risk altering the original

	clone, err := copystructure.Copy(d.Config)
	if err != nil {
		Logc(ctx).Errorf("Could not clone GCP backend config; %v", err)
		return drivers.GCPNFSStorageDriverConfig{}
	}

	cloneConfig, ok := clone.(drivers.GCPNFSStorageDriverConfig)
	if !ok {
		Logc(ctx).Errorf("Could not cast GCP backend config; %v", err)
		return drivers.GCPNFSStorageDriverConfig{}
	}

	cloneConfig.APIKey = drivers.GCPPrivateKey{
		Type:                    utils.REDACTED,
		ProjectID:               utils.REDACTED,
		PrivateKeyID:            utils.REDACTED,
		PrivateKey:              utils.REDACTED,
		ClientEmail:             utils.REDACTED,
		ClientID:                utils.REDACTED,
		AuthURI:                 utils.REDACTED,
		TokenURI:                utils.REDACTED,
		AuthProviderX509CertURL: utils.REDACTED,
		ClientX509CertURL:       utils.REDACTED,
	}
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: utils.REDACTED,
		drivers.KeyType: utils.REDACTED,
	} // redact the credentials
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

// String implements stringer interface for the NFSStorageDriver driver
func (d NFSStorageDriver) String() string {
	// Cannot use GetExternalConfig as it contains log statements
	return utils.ToStringRedacted(&d, []string{"API"}, nil)
}

// GoString implements GoStringer interface for the NFSStorageDriver driver
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
func (d *NFSStorageDriver) getVolumeExternal(volumeAttrs *api.Volume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            volumeAttrs.Name,
		InternalName:    volumeAttrs.CreationToken,
		Size:            strconv.FormatInt(volumeAttrs.QuotaInBytes, 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     strconv.FormatBool(volumeAttrs.SnapshotDirectory),
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
		ServiceLevel:    api.UserServiceLevelFromAPIServiceLevel(volumeAttrs.ServiceLevel),
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

	if !drivers.AreSameCredentials(d.Config.Credentials, dOrig.Config.Credentials) {
		bitmap.Add(storage.CredentialsChange)
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
	matched, err := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9-]{0,70}$`, storagePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		return fmt.Errorf("storage prefix may only contain letters/digits/hyphens and must begin with a letter")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d NFSStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}
