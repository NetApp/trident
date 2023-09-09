// Copyright 2022 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/mitchellh/copystructure"
	"go.uber.org/multierr"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	MinimumVolumeSizeBytes       = uint64(1073741824)   // 1 GiB
	MinimumCVSVolumeSizeBytesHW  = uint64(107374182400) // 100 GiB
	MaximumVolumesPerStoragePool = 50
	MinimumAPIVersion            = "1.4.0"
	MinimumSDEVersion            = "2023.1.2"

	defaultHWServiceLevel  = api.UserServiceLevel1
	defaultSWServiceLevel  = api.PoolServiceLevel1
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
	StoragePools    = "storagePools"

	// Topology label names
	topologyZoneLabel   = drivers.TopologyLabelPrefix + "/" + Zone
	topologyRegionLabel = drivers.TopologyLabelPrefix + "/" + Region

	// discovery debug log constant
	discovery = "discovery"
)

// NFSStorageDriver is for storage provisioning using Cloud Volumes Service in GCP
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.GCPNFSStorageDriverConfig
	API                 api.GCPClient
	telemetry           *Telemetry
	apiVersion          *versionutils.Version
	sdeVersion          *versionutils.Version
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
	return tridentconfig.GCPNFSStorageDriverName
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
	fields := LogFields{"Method": "Initialize", "Type": "NFSStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	commonConfig.DriverContext = context
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	d.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := d.initializeGCPConfig(ctx, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver. %v", d.Name(), err)
	}
	d.Config = *config

	if err = d.populateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	d.initializeStoragePools(ctx)

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		d.API = d.initializeGCPAPIClient(ctx, &d.Config)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %v", err)
	}
	d.Config.BackendPools = pools

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
	fields := LogFields{"Method": "Terminate", "Type": "NFSStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NFSStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.GCPNFSStorageDriverConfig,
) error {
	fields := LogFields{"Method": "populateConfigurationDefaults", "Type": "NFSStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> populateConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< populateConfigurationDefaults")

	if config.StoragePrefix == nil {
		defaultPrefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		defaultPrefix = strings.Replace(defaultPrefix, "_", "-", -1)
		config.StoragePrefix = &defaultPrefix
	}

	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	}

	if config.ServiceLevel == "" {
		if config.StorageClass == api.StorageClassSoftware {
			config.ServiceLevel = defaultSWServiceLevel
		} else {
			config.ServiceLevel = defaultHWServiceLevel
		}
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

	Logc(ctx).WithFields(LogFields{
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
func (d *NFSStorageDriver) initializeStoragePools(ctx context.Context) {
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
		pool.InternalAttributes()[ServiceLevel] = strings.ToLower(d.Config.ServiceLevel)
		pool.InternalAttributes()[StorageClass] = d.Config.StorageClass
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes()[UnixPermissions] = d.Config.UnixPermissions
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[Network] = d.Config.Network
		pool.InternalAttributes()[Region] = d.Config.Region
		pool.InternalAttributes()[Zone] = d.Config.Zone
		pool.InternalAttributes()[StoragePools] = strings.Join(d.Config.StoragePools, ",")

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

			storagePools := d.Config.StoragePools
			if vpool.StoragePools != nil {
				storagePools = vpool.StoragePools
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
			pool.InternalAttributes()[ServiceLevel] = strings.ToLower(serviceLevel)
			pool.InternalAttributes()[StorageClass] = storageClass
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[Network] = network
			pool.InternalAttributes()[Region] = region
			pool.InternalAttributes()[Zone] = zone
			pool.InternalAttributes()[StoragePools] = strings.Join(storagePools, ",")

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}
}

// initializeGCPConfig parses the GCP config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeGCPConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.GCPNFSStorageDriverConfig, error) {
	fields := LogFields{"Method": "initializeGCPConfig", "Type": "NFSStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeGCPConfig")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeGCPConfig")

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
) api.GCPClient {
	fields := LogFields{"Method": "initializeGCPAPIClient", "Type": "NFSStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeGCPAPIClient")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeGCPAPIClient")

	client := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		APIURL:          config.APIURL,
		APIAudienceURL:  config.APIAudienceURL,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	return client
}

// validate ensures the driver configuration and execution environment are valid and working
func (d *NFSStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NFSStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

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

	if d.apiVersion.LessThan(versionutils.MustParseSemantic(MinimumAPIVersion)) {
		return fmt.Errorf("API version is %s, at least %s is required", d.apiVersion.String(), MinimumAPIVersion)
	}
	if d.sdeVersion.LessThan(versionutils.MustParseSemantic(MinimumSDEVersion)) {
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

		// Validate storage class
		if !api.IsValidStorageClass(pool.InternalAttributes()[StorageClass]) {
			return fmt.Errorf("invalid storage class in pool %s: %s", poolName, pool.InternalAttributes()[StorageClass])
		}

		// Validate service level
		if !api.IsValidUserServiceLevel(pool.InternalAttributes()[ServiceLevel], pool.InternalAttributes()[StorageClass]) {
			return fmt.Errorf("invalid service level in pool %s: %s", poolName, pool.InternalAttributes()[ServiceLevel])
		}

		// Validate storagePools field
		if pool.InternalAttributes()[StorageClass] == api.StorageClassHardware &&
			pool.InternalAttributes()[StoragePools] != "" {
			return fmt.Errorf("storagePools not expected for hardware type storage class pool %s: %s",
				poolName, pool.InternalAttributes()[StoragePools])
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

	fields := LogFields{
		"Method": "Create",
		"Type":   "NFSStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

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
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Take storage class from volume config (named CVSStorageClass to avoid name collision)
	// first (handles Docker case), then from pool.
	storageClass := volConfig.CVSStorageClass
	if storageClass == "" {
		storageClass = pool.InternalAttributes()[StorageClass]
	}

	unixPermissions := volConfig.UnixPermissions
	if unixPermissions == "" {
		unixPermissions = pool.InternalAttributes()[UnixPermissions]
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
	sizeBytes := requestedSizeBytes
	if storageClass == api.StorageClassHardware {
		sizeBytes = d.applyMinimumVolumeSizeHW(requestedSizeBytes)
	}

	if requestedSizeBytes < sizeBytes {
		Logc(ctx).WithFields(LogFields{
			"name":          name,
			"requestedSize": requestedSizeBytes,
			"minimumSize":   sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable.")
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	userServiceLevel := volConfig.ServiceLevel
	if userServiceLevel == "" {
		userServiceLevel = pool.InternalAttributes()[ServiceLevel]
	}
	if !api.IsValidUserServiceLevel(userServiceLevel, storageClass) {
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
	}

	// Take network from volume config first (handles Docker case), then from pool
	network := volConfig.Network
	if network == "" {
		network = pool.InternalAttributes()[Network]
	}
	gcpNetwork := d.makeNetworkPath(network)

	// TODO: Remove when software storage class allows NFSv4
	protocolTypes := []string{api.ProtocolTypeNFSv3}
	if storageClass == api.StorageClassHardware {
		protocolTypes = append(protocolTypes, api.ProtocolTypeNFSv4)
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

	// Update config to reflect values used to create volume
	volConfig.CVSStorageClass = storageClass
	volConfig.UnixPermissions = unixPermissions
	volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	volConfig.ServiceLevel = userServiceLevel
	volConfig.SnapshotDir = snapshotDir
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.Network = network
	volConfig.Zone = zone

	Logc(ctx).WithFields(LogFields{
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
		UnixPermissions:   unixPermissions,
		Network:           gcpNetwork,
		Zone:              zone,
	}

	if storageClass == api.StorageClassSoftware {
		return d.createSOVolume(ctx, createRequest, volConfig, pool)
	}

	return d.createVolume(ctx, createRequest, volConfig)
}

// createSOVolume gets the GCP storage pools and tries to create software storage class volume in one of the pools.
func (d *NFSStorageDriver) createSOVolume(
	ctx context.Context, createRequest *api.VolumeCreateRequest, config *storage.VolumeConfig, sPool storage.Pool,
) error {
	GCPPools, err := d.GetPoolsForCreate(ctx, sPool, config.ServiceLevel, createRequest.QuotaInBytes)
	if err != nil {
		return err
	}

	if config.ServiceLevel == api.PoolServiceLevel2 {
		// This field is set for Zone Redundant Pools only
		createRequest.RegionalHA = true
	}

	var createErrors error
	for _, GCPPool := range GCPPools {
		createRequest.PoolID = GCPPool.PoolID

		err := d.createVolume(ctx, createRequest, config)
		if errors.IsVolumeCreatingError(err) {
			// For this case, volume create will be retried in the future
			// So we return here itself instead of trying for another pool
			return err
		}
		if err != nil {
			createErrors = multierr.Append(createErrors, err)
			continue
		}

		return nil
	}

	return createErrors
}

// createVolume makes the API call to create a volume and waits till the volume create state changes
// from creating to some other state. It is common for SO and PO volumes.
func (d *NFSStorageDriver) createVolume(
	ctx context.Context, createRequest *api.VolumeCreateRequest, config *storage.VolumeConfig,
) error {
	formatErr := func(msg string, err error) error {
		if createRequest.PoolID != "" {
			createErr := fmt.Errorf("GCP pool %s, %s: %s", createRequest.PoolID, msg, err)
			Logc(ctx).Error(createErr)
			return createErr
		}
		createErr := fmt.Errorf("%s: %s", msg, err)
		Logc(ctx).Error(createErr)
		return createErr
	}

	// Create the volume
	if err := d.API.CreateVolume(ctx, createRequest); err != nil {
		return formatErr("error creating volume", err)
	}

	// Get the volume
	newVolume, err := d.API.GetVolumeByCreationToken(ctx, createRequest.CreationToken)
	if err != nil {
		return formatErr("error getting new volume", err)
	}

	// Always save the ID so, we can find the volume efficiently later
	config.InternalID = d.CreateGCPInternalID(newVolume)

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, newVolume, createRequest.CreationToken)
}

func (d *NFSStorageDriver) ensureTopologyRegionAndZone(
	ctx context.Context, volConfig *storage.VolumeConfig, storageClass, configuredZone string,
) string {
	// Check if configured topology (zone/region) matches provided topology for software volumes
	// If zone is not configured, use the preferred zone from the topology
	if len(volConfig.PreferredTopologies) > 0 {
		if r, ok := volConfig.PreferredTopologies[0][topologyRegionLabel]; ok {
			if d.Config.APIRegion != r {
				Logc(ctx).WithFields(LogFields{
					"configuredRegion": d.Config.APIRegion,
					"topologyRegion":   r,
				}).Warn("configured region does not match topology region")
			}
		}
		if storageClass == api.StorageClassSoftware {
			if z, ok := volConfig.PreferredTopologies[0][topologyZoneLabel]; ok {
				if configuredZone != "" && configuredZone != z {
					Logc(ctx).WithFields(LogFields{
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
	ctx context.Context, sourceVolConfig, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":   "CreateClone",
		"Type":     "NFSStorageDriver",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	// ensure new volume doesn't exist, fail if so
	// get source volume, fail if nonexistent or if wrong region
	// if snapshot specified, read snapshot from source, fail if nonexistent
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
		switch extantVolume.LifeCycleState {

		case api.StateCreating, api.StateRestoring:
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(fmt.Sprintf("volume state is %s, not %s",
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

	var sourceSnapshot *api.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		if sourceSnapshot, err = d.API.GetSnapshotForVolume(ctx, sourceVolume, snapshot); err != nil {
			return fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.LifeCycleState != api.StateAvailable {
			return fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.LifeCycleState, api.StateAvailable)
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapshot,
			"source":   sourceVolume.Name,
		}).Debug("Found source snapshot.")

	} else {
		// No source snapshot specified, so create one
		snapName := time.Now().UTC().Format(storage.SnapshotNameFormat)

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapName,
			"source":   sourceVolume.Name,
		}).Debug("Creating source snapshot.")

		snapshotCreateRequest := &api.SnapshotCreateRequest{
			VolumeID: sourceVolume.VolumeID,
			Name:     snapName,
		}

		if err = d.API.CreateSnapshot(ctx, snapshotCreateRequest); err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		if sourceSnapshot, err = d.API.GetSnapshotForVolume(ctx, sourceVolume, snapName); err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		if err = d.API.WaitForSnapshotState(
			ctx, sourceSnapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout()); err != nil {
			return err
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot": sourceSnapshot.Name,
			"source":   sourceVolume.Name,
		}).Debug("Created source snapshot.")
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

	Logc(ctx).WithFields(LogFields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshot.Name,
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
		SnapshotID:        sourceSnapshot.SnapshotID,
		UnixPermissions:   sourceVolume.UnixPermissions,
		PoolID:            sourceVolume.PoolID,
		Network:           gcpNetwork,
	}

	// Use the same service level for clone as the source volume.
	cloneVolConfig.ServiceLevel = sourceVolConfig.ServiceLevel

	// Check if we need to create clone in zone redundant pool
	if cloneVolConfig.ServiceLevel == api.PoolServiceLevel2 {
		createRequest.RegionalHA = true
	}

	// Clone the volume and wait for the creation to complete
	return d.createVolume(ctx, createRequest, cloneVolConfig)
}

func (d *NFSStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "NFSStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Get the volume
	creationToken := originalName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	// Set the serviceLevel in the imported volumes's config appropriately.
	// a) for storageClass == software,
	//      i) if RegionalHA is true, this means the volume is created in zoneRedundantstandardsw pool
	//      ii) else use the serviceLevel as "standardsw"
	// b) for storageClass == hardware, user save the servicelevel as one of standard, premium and extreme

	if volume.StorageClass == api.StorageClassSoftware {
		if volume.RegionalHA {
			volConfig.ServiceLevel = api.PoolServiceLevel2
		} else {
			volConfig.ServiceLevel = api.PoolServiceLevel1
		}
	} else {
		volConfig.ServiceLevel = api.UserServiceLevelFromAPIServiceLevel(volume.ServiceLevel)
	}

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

	// Always save the ID so, we can find the volume efficiently later
	volConfig.InternalID = d.CreateGCPInternalID(volume)

	return nil
}

func (d *NFSStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "NFSStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

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
			Logc(ctx).WithFields(LogFields{
				"volume": volumeName,
			}).Debugf("Volume is in %s state.", state)
			return errors.VolumeCreatingError(err.Error())
		}

		// Don't leave a CVS volume in a non-transitional state laying around in error state
		if !api.IsTransitionalState(state) {
			errDelete := d.API.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(LogFields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDelete)
			} else {
				// Wait for deletion to complete
				_, errDeleteWait := d.API.WaitForVolumeStates(
					ctx, volume, []string{api.StateDeleted}, []string{api.StateError}, d.defaultTimeout())
				if errDeleteWait != nil {
					Logc(ctx).WithFields(LogFields{
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
	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NFSStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

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
	fields := LogFields{
		"Method": "Publish",
		"Type":   "NFSStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Get the volume
	creationToken := name

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Heal the ID on legacy volumes
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateGCPInternalID(volume)
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

	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NFSStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

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
	return d.getSnapshot(ctx, extantVolume, snapConfig)
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

			created := snapshot.Created.UTC().Format(utils.TimestampFormat)

			Logc(ctx).WithFields(LogFields{
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

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NFSStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NFSStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	// Get the volume
	creationToken := internalVolName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}
	return d.getSnapshots(ctx, volume, volConfig)
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
			Created:   snapshot.Created.Format(utils.TimestampFormat),
			SizeBytes: volume.QuotaInBytes,
			State:     storage.SnapshotStateOnline,
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

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NFSStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

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
	return d.createSnapshot(ctx, sourceVolume, snapConfig)
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

	Logc(ctx).WithFields(LogFields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Info("Snapshot created.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.Format(utils.TimestampFormat),
		SizeBytes: sourceVolume.QuotaInBytes,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NFSStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NFSStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	// Get the volume
	creationToken := internalVolName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Get the snapshot
	snapshot, err := d.API.GetSnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	// Do the restore
	return d.API.RestoreSnapshot(ctx, volume, snapshot)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NFSStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NFSStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

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
	return d.deleteSnapshot(ctx, extantVolume, snapConfig)
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

// List returns the list of volumes associated with this tenant
func (d *NFSStorageDriver) List(ctx context.Context) ([]string, error) {
	fields := LogFields{"Method": "List", "Type": "NFSStorageDriver"}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> List")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< List")

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
	fields := LogFields{"Method": "Get", "Type": "NFSStorageDriver"}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	_, err := d.API.GetVolumeByCreationToken(ctx, name)

	return err
}

// Resize increases a volume's quota
func (d *NFSStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":    "Resize",
		"Type":      "NFSStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Get the volume
	creationToken := name

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Heal the ID on legacy volumes
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateGCPInternalID(volume)
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
		return errors.UnsupportedCapacityRangeError(fmt.Errorf("requested size %d is less than existing volume size %d",
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

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *NFSStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.GCPNFSStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NFSStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. Project number
	// 2. API region
	// 3. Service type
	// 4. Storage pool - contains at least one pool depending on the backend configuration.
	backendPools := make([]drivers.GCPNFSStorageBackendPool, 0)
	for _, pool := range d.pools {
		backendPool := drivers.GCPNFSStorageBackendPool{
			ProjectNumber: d.Config.ProjectNumber,
			APIRegion:     d.Config.APIRegion,
			ServiceLevel:  d.Config.ServiceLevel,
			StoragePool:   pool.Name(),
		}
		backendPools = append(backendPools, backendPool)
	}
	return backendPools
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
	fields := LogFields{
		"Method": "CreateFollowup",
		"Type":   "NFSStorageDriver",
		"name":   volConfig.InternalName,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

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
	for idx := range *volumes {
		volume := &(*volumes)[idx]

		// Filter out volumes in an unavailable state
		switch volume.LifeCycleState {
		case api.StateDisabled, api.StateDeleting, api.StateDeleted, api.StateError:
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.CreationToken, prefix) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(volume), Error: nil}
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

func (d *NFSStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, _, _ string) error {
	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NFSStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return nil
}

func validateStoragePrefix(storagePrefix string) error {
	matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9-]{0,70}$`, storagePrefix)
	if !matched {
		return fmt.Errorf("storage prefix may only contain letters/digits/hyphens and must begin with a letter")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d NFSStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// getGCPPoolMap gets all the GCP pools for particular project and returns map
// of <GCP pool ID, *GCP pool>.
func (d *NFSStorageDriver) getGCPPoolMap(ctx context.Context) (map[string]*api.Pool, error) {
	pools, err := d.API.GetPools(ctx)
	if err != nil {
		Logc(ctx).Error("Failed to get GCP pools", err)
		return nil, err
	}

	poolMap := make(map[string]*api.Pool, len(*pools))
	for _, pool := range *pools {
		poolMap[pool.PoolID] = pool
	}

	return poolMap, nil
}

// GetPoolsForCreate is called by software storage class backends only.
// We return an array of storage pools. If no pools are found, we return error.
func (d *NFSStorageDriver) GetPoolsForCreate(
	ctx context.Context, sPool storage.Pool, poolServiceLevel string, volSizeBytes int64,
) ([]*api.Pool, error) {
	GCPPools, poolErrors, err := d.GetGCPPoolsForStoragePool(ctx, sPool, poolServiceLevel, volSizeBytes)
	if err != nil {
		return nil, err
	}

	// Software class backend doesn't have pools with matching filters, so we throw error
	if len(GCPPools) == 0 {
		if poolErrors != "" {
			return nil, errors.ResourceExhaustedError(fmt.Errorf(poolErrors))
		}
		return nil, fmt.Errorf("no GCP pools found")
	}

	// Sort GCP pools on number of volumes
	sort.Slice(GCPPools, func(i, j int) bool {
		return GCPPools[i].NumberOfVolumes < GCPPools[j].NumberOfVolumes
	})

	return GCPPools, nil
}

// GetGCPPoolsForStoragePool returns all discovered GCP pools matching the specified
// storage pool and service level.
func (d *NFSStorageDriver) GetGCPPoolsForStoragePool(
	ctx context.Context, sPool storage.Pool, poolServiceLevel string, volSizeBytes int64,
) ([]*api.Pool, string, error) {
	if d.Config.DebugTraceFlags[discovery] {
		Logc(ctx).WithField("storagePool", sPool.Name()).Debugf("Determining capacity pools for storage pool.")
	}

	GCPPoolMap, err := d.getGCPPoolMap(ctx)
	if err != nil {
		return nil, "", err
	}

	// Map to track GCPPools that passes the filters
	filteredGCPPoolMap := make(map[string]bool, len(GCPPoolMap))
	for poolID := range GCPPoolMap {
		filteredGCPPoolMap[poolID] = true
	}

	// If GCP pools were specified in backend, filter out non-matching GCP pools
	poolList := utils.SplitString(ctx, sPool.InternalAttributes()[StoragePools], ",")
	if len(poolList) > 0 {
		for poolID := range filteredGCPPoolMap {
			if !utils.SliceContainsString(poolList, poolID) {
				if d.Config.DebugTraceFlags[discovery] {
					Logc(ctx).Debugf("Ignoring GCP pool %s, not in storage pools [%s].", poolID, poolList)
				}
				filteredGCPPoolMap[poolID] = false
			}
		}
	}

	// Filter out pools with non-matching service levels
	for _, pool := range GCPPoolMap {
		if !strings.EqualFold(pool.ServiceLevel, poolServiceLevel) {
			if d.Config.DebugTraceFlags[discovery] {
				Logc(ctx).Debugf("Ignoring GCP pool %s, not in service level [%s].", pool.PoolID, poolServiceLevel)
			}
			filteredGCPPoolMap[pool.PoolID] = false
		}
	}

	// Filter out pools with insufficient resources
	var poolErrors []string
	for _, pool := range GCPPoolMap {
		if pool.NumberOfVolumes >= MaximumVolumesPerStoragePool {
			errMsg := fmt.Sprintf("Ignoring GCP pool %s, volume limit reached.", pool.PoolID)
			if d.Config.DebugTraceFlags[discovery] {
				Logc(ctx).Debugf(errMsg)
			}
			if utils.SliceContainsString(poolList, pool.PoolID) {
				poolErrors = append(poolErrors, errMsg)
			}
			filteredGCPPoolMap[pool.PoolID] = false
			continue
		}
		if pool.AvailableCapacity() < volSizeBytes {
			errMsg := fmt.Sprintf("Ignoring GCP pool %s, unsupported capacity range.", pool.PoolID)
			if d.Config.DebugTraceFlags[discovery] {
				Logc(ctx).Debugf(errMsg)
			}
			if utils.SliceContainsString(poolList, pool.PoolID) {
				poolErrors = append(poolErrors, errMsg)
			}
			filteredGCPPoolMap[pool.PoolID] = false
		}
	}

	// Build list of all GCP pools that have passed all filters
	filteredGCPPoolList := make([]*api.Pool, 0)
	for poolID, match := range filteredGCPPoolMap {
		if match {
			filteredGCPPoolList = append(filteredGCPPoolList, GCPPoolMap[poolID])
		}
	}

	return filteredGCPPoolList, strings.Join(poolErrors, ", "), nil
}

func (d *NFSStorageDriver) CreateGCPInternalID(vol *api.Volume) string {
	if vol.PoolID != "" {
		return fmt.Sprintf("/gcpProject/%s/poolID/%s/volumeID/%s", d.Config.ProjectNumber, vol.PoolID, vol.VolumeID)
	}
	return fmt.Sprintf("/gcpProject/%s/poolID/none/volumeID/%s", d.Config.ProjectNumber, vol.VolumeID)
}
