// Copyright 2025 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/uuid"
	"go.uber.org/multierr"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/gcnvapi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nfs"
)

const (
	MinimumGCNVVolumeSizeBytesSW = uint64(1073741824)   // 1 GiB
	MinimumGCNVVolumeSizeBytesHW = uint64(107374182400) // 100 GiB

	defaultVolumeSizeStr = "107374182400"

	// Constants for internal pool attributes

	CapacityPools = "capacityPools"

	nfsVersion3  = "3"
	nfsVersion4  = "4"
	nfsVersion41 = "4.1"
)

var (
	supportedNFSVersions     = []string{nfsVersion3, nfsVersion4, nfsVersion41}
	storagePrefixRegex       = regexp.MustCompile(`^$|^[a-zA-Z][a-zA-Z-]*$`)
	volumeNameRegex          = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	volumeCreationTokenRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,78}[a-z0-9])?$`)
	gcpLabelRegex            = regexp.MustCompile(`[^-_a-z0-9\p{L}]`)
	csiRegex                 = regexp.MustCompile(`^pvc-[\da-fA-F]{8}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{12}$`)
	nfsMountPathRegex        = regexp.MustCompile(`^(?P<server>.+):/(?P<share>.+)$`)
	smbMountPathRegex        = regexp.MustCompile(`\\\\(?P<server>.+)\\(?P<share>.+)$`)
)

// NASStorageDriver is for storage provisioning using the Google Cloud NetApp Volumes service.
type NASStorageDriver struct {
	initialized         bool
	Config              drivers.GCNVNASStorageDriverConfig
	API                 gcnvapi.GCNV
	telemetry           *Telemetry
	pools               map[string]storage.Pool
	volumeCreateTimeout time.Duration
}

// Name returns the name of this driver.
func (d *NASStorageDriver) Name() string {
	return tridentconfig.GCNVNASStorageDriverName
}

// GetConfig returns the config of this driver.
func (d *NASStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

// defaultBackendName returns the default name of the backend managed by this driver instance.
func (d *NASStorageDriver) defaultBackendName() string {
	id := crypto.RandomString(5)
	if len(d.Config.APIKey.PrivateKeyID) > 5 {
		id = d.Config.APIKey.PrivateKeyID[0:5]
	}
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), id)
}

// BackendName returns the name of the backend managed by this driver instance.
func (d *NASStorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		// Use the old naming scheme if no name is specified
		return d.defaultBackendName()
	}
}

// poolName constructs the name of the pool reported by this driver instance.
func (d *NASStorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// validateVolumeName checks that the name of a new volume matches the requirements of a GCNV volume name.
func (d *NASStorageDriver) validateVolumeName(name string) error {
	if !volumeNameRegex.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be 1-63 characters long, "+
			"begin with a letter, not end with a hyphen, and contain only letters, digits, and hyphens", name)
	}
	return nil
}

// validateCreationToken checks that the creation token of a new volume matches the requirements of a creation token.
func (d *NASStorageDriver) validateCreationToken(name string) error {
	if !volumeCreationTokenRegex.MatchString(name) {
		return fmt.Errorf("volume internal name '%s' is not allowed; it must be 1-80 characters long, "+
			"begin with a letter, not end with a hyphen, and contain only letters, digits, and hyphens", name)
	}
	return nil
}

// defaultCreateTimeout sets the driver timeout for volume create/delete operations.  Docker gets more time, since
// it doesn't have a mechanism to retry.
func (d *NASStorageDriver) defaultCreateTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerCreateTimeout
	default:
		return gcnvapi.VolumeCreateTimeout
	}
}

// defaultTimeout controls the driver timeout for most workflows.
func (d *NASStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return gcnvapi.DefaultTimeout
	}
}

// Initialize initializes this driver from the provided config.
func (d *NASStorageDriver) Initialize(
	ctx context.Context, context tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "NASStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< Initialize")

	commonConfig.DriverContext = context

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := d.initializeGCNVConfig(ctx, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver. %v", d.Name(), err)
	}
	d.Config = *config

	if err = d.populateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	d.initializeStoragePools(ctx)
	d.initializeTelemetry(ctx, backendUUID)

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = d.initializeGCNVAPIClient(ctx, &d.Config); err != nil {
			return fmt.Errorf("error validating %s GCNV API. %v", d.Name(), err)
		}
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

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":              *config.StoragePrefix,
		"Size":                       config.Size,
		"ServiceLevel":               config.ServiceLevel,
		"NfsMountOptions":            config.NFSMountOptions,
		"LimitVolumeSize":            config.LimitVolumeSize,
		"ExportRule":                 config.ExportRule,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	})

	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated).
func (d *NASStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded.
func (d *NASStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NASStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.GCNVNASStorageDriverConfig,
) error {
	fields := LogFields{"Method": "populateConfigurationDefaults", "Type": "NASStorageDriver"}
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
		config.ServiceLevel = gcnvapi.ServiceLevelStandard
	}

	if config.StorageClass == "" {
		config.StorageClass = defaultStorageClass
	}

	if config.SnapshotDir != "" {
		// Set the snapshotDir provided in the config
		snapDirFormatted, err := convert.ToFormattedBool(config.SnapshotDir)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir: %v.", config.SnapshotDir)
			return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
		}
		config.SnapshotDir = snapDirFormatted
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

	if config.NASType == "" {
		config.NASType = sa.NFS
	}

	// VolumeCreateTimeoutSeconds is the timeout value in seconds.
	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		i, err := strconv.ParseInt(d.Config.VolumeCreateTimeout, 10, 64)
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
		"NFSMountOptions":            config.NFSMountOptions,
		"SnapshotDir":                config.SnapshotDir,
		"SnapshotReserve":            config.SnapshotReserve,
		"LimitVolumeSize":            config.LimitVolumeSize,
		"ExportRule":                 config.ExportRule,
		"NetworkName":                config.Network,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	}).Debugf("Configuration defaults")

	return nil
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NASStorageDriver) initializeStoragePools(ctx context.Context) {
	d.pools = make(map[string]storage.Pool)

	if len(d.Config.Storage) == 0 {

		Logc(ctx).Debug("No vpools defined, reporting single pool.")

		// No vpools defined, so report region/zone as a single pool
		pool := storage.NewStoragePool(nil, d.poolName("pool"))

		pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
		pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
		pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(true)
		pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)
		pool.Attributes()[sa.NASType] = sa.NewStringOffer(d.Config.NASType)

		if d.Config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes()[Size] = d.Config.Size
		pool.InternalAttributes()[UnixPermissions] = d.Config.UnixPermissions
		pool.InternalAttributes()[ServiceLevel] = convert.ToTitle(d.Config.ServiceLevel)
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[Network] = d.Config.Network
		pool.InternalAttributes()[CapacityPools] = strings.Join(d.Config.StoragePools, ",")

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

			unixPermissions := d.Config.UnixPermissions
			if vpool.UnixPermissions != "" {
				unixPermissions = vpool.UnixPermissions
			}

			supportedTopologies := d.Config.SupportedTopologies
			if vpool.SupportedTopologies != nil {
				supportedTopologies = vpool.SupportedTopologies
			}

			capacityPools := d.Config.StoragePools
			if vpool.StoragePools != nil {
				capacityPools = vpool.StoragePools
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

			network := d.Config.Network
			if vpool.Network != "" {
				network = vpool.Network
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
			pool.Attributes()[sa.NASType] = sa.NewStringOffer(d.Config.NASType)

			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[ServiceLevel] = convert.ToTitle(serviceLevel)
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[Network] = network
			pool.InternalAttributes()[CapacityPools] = strings.Join(capacityPools, ",")

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}

	return
}

// initializeTelemetry assembles all the telemetry data to be used as volume labels.
func (d *NASStorageDriver) initializeTelemetry(_ context.Context, backendUUID string) {
	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
	}
}

// initializeGCNVConfig parses the GCNV config, mixing in the specified common config.
func (d *NASStorageDriver) initializeGCNVConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.GCNVNASStorageDriverConfig, error) {
	fields := LogFields{"Method": "initializeGCNVConfig", "Type": "NASStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> initializeGCNVConfig")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< initializeGCNVConfig")

	config := &drivers.GCNVNASStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into GCNVNASStorageDriverConfig object
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

// initializeGCNVAPIClient returns a GCNV API client.
func (d *NASStorageDriver) initializeGCNVAPIClient(
	ctx context.Context, config *drivers.GCNVNASStorageDriverConfig,
) (gcnvapi.GCNV, error) {
	fields := LogFields{"Method": "initializeGCNVAPIClient", "Type": "NASStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> initializeGCNVAPIClient")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< initializeGCNVAPIClient")

	sdkTimeout := gcnvapi.DefaultSDKTimeout
	if config.SDKTimeout != "" {
		if i, parseErr := strconv.ParseInt(d.Config.SDKTimeout, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.SDKTimeout).WithError(parseErr).Error(
				"Invalid value for SDK timeout.")
			return nil, parseErr
		} else {
			sdkTimeout = time.Duration(i) * time.Second
		}
	}

	maxCacheAge := gcnvapi.DefaultMaxCacheAge
	if config.MaxCacheAge != "" {
		if i, parseErr := strconv.ParseInt(d.Config.MaxCacheAge, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.MaxCacheAge).WithError(parseErr).Error(
				"Invalid value for max cache age.")
			return nil, parseErr
		} else {
			maxCacheAge = time.Duration(i) * time.Second
		}
	}

	gcnv, err := gcnvapi.NewDriver(ctx, &gcnvapi.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		Location:        config.Location,
		APIKey:          &config.APIKey,
		DebugTraceFlags: config.DebugTraceFlags,
		SDKTimeout:      sdkTimeout,
		MaxCacheAge:     maxCacheAge,
	})
	if err != nil {
		return nil, err
	}

	// The storage pools should already be set up by this point. We register the pools with the
	// API layer to enable matching of storage pools with discovered GCNV resources.
	if err = gcnv.Init(ctx, d.pools); err != nil {
		return nil, err
	}

	return gcnv, nil
}

// validate ensures the driver configuration and execution environment are valid and working.
func (d *NASStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	// Ensure storage prefix is compatible with cloud service
	if err := validateGCNVStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level (it is allowed to be blank)
		serviceLevel := pool.InternalAttributes()[ServiceLevel]
		switch serviceLevel {
		case gcnvapi.ServiceLevelFlex, gcnvapi.ServiceLevelStandard,
			gcnvapi.ServiceLevelPremium, gcnvapi.ServiceLevelExtreme, "":
			break
		default:
			return fmt.Errorf("invalid service level in pool %s: %s",
				poolName, pool.InternalAttributes()[ServiceLevel])
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
			if _, err := strconv.ParseBool(pool.InternalAttributes()[SnapshotDir]); err != nil {
				return fmt.Errorf("invalid boolean value for snapshotDir in pool %s; %v", poolName, err)
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

		// Validate unix permissions
		if pool.InternalAttributes()[UnixPermissions] != "" {
			if err := filesystem.ValidateOctalUnixPermissions(pool.InternalAttributes()[UnixPermissions]); err != nil {
				return fmt.Errorf("invalid value for unixPermissions in pool %s; %v", poolName, err)
			}
		}

		// Validate default size
		if _, err := capacity.ToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s; %v", poolName, err)
		}
	}

	return nil
}

// Create creates a new volume.
func (d *NASStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Create",
		"Type":   "NASStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Make sure we got a valid name
	if err := d.validateVolumeName(volConfig.Name); err != nil {
		return err
	}

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(name); err != nil {
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
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		if extantVolume.State == gcnvapi.VolumeStateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", gcnvapi.VolumeStateCreating, gcnvapi.VolumeStateReady))
		}

		Logc(ctx).WithFields(LogFields{
			"name":  name,
			"state": extantVolume.State,
		}).Warning("Volume already exists.")

		return drivers.NewVolumeExistsError(name)
	}

	// Take service level from volume config first (handles Docker case), then from pool.
	// Service level should not be empty at this point due to application of config defaults.
	// The service level is needed to select the minimum allowable volume size.
	serviceLevel := convert.ToTitle(volConfig.ServiceLevel)
	if serviceLevel == "" {
		serviceLevel = pool.InternalAttributes()[ServiceLevel]
	}

	minimumGCNVVolumeSizeBytes := MinimumGCNVVolumeSizeBytesHW
	if serviceLevel == gcnvapi.ServiceLevelFlex {
		minimumGCNVVolumeSizeBytes = MinimumGCNVVolumeSizeBytesSW
	}

	// Take snapshot reserve from volume config first (handles Docker case), then from pool
	snapshotReserve := volConfig.SnapshotReserve
	if snapshotReserve == "" {
		snapshotReserve = pool.InternalAttributes()[SnapshotReserve]
	}
	var snapshotReservePtr *int64
	var snapshotReserveInt int
	if snapshotReserve != "" {
		snapshotReserveInt64, err := strconv.ParseInt(snapshotReserve, 10, 0)
		if err != nil {
			return fmt.Errorf("invalid value for snapshotReserve: %v", err)
		}
		snapshotReserveInt = int(snapshotReserveInt64)
		snapshotReservePtr = &snapshotReserveInt64
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s; %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size; %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := capacity.ToBytes(pool.InternalAttributes()[Size])
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if err = drivers.CheckMinVolumeSize(sizeBytes, MinimumVolumeSizeBytes); err != nil {
		return err
	}

	if sizeBytes < minimumGCNVVolumeSizeBytes {

		Logc(ctx).WithFields(LogFields{
			"name":          name,
			"requestedSize": sizeBytes,
			"minimumSize":   minimumGCNVVolumeSizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable.")

		sizeBytes = minimumGCNVVolumeSizeBytes
	}

	// Get the volume size based on the snapshot reserve
	sizeWithReserveBytes := drivers.CalculateVolumeSizeBytes(ctx, name, sizeBytes, snapshotReserveInt)

	_, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeWithReserveBytes, d.Config.CommonStorageDriverConfig)
	if err != nil {
		return err
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NFSMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Take unix permissions from volume config first (handles Docker case & PVC annotations), then from pool
	unixPermissions := volConfig.UnixPermissions
	if unixPermissions == "" {
		unixPermissions = pool.InternalAttributes()[UnixPermissions]
	}

	// Determine protocol from mount options
	var protocolTypes []string
	var smbAccess, nfsV3Access, nfsV41Access bool
	var apiExportRule gcnvapi.ExportRule
	var exportPolicy *gcnvapi.ExportPolicy
	var nfsVersion string

	if d.Config.NASType == sa.SMB {
		protocolTypes = []string{gcnvapi.ProtocolTypeSMB}
	} else {
		nfsVersion, err = nfs.GetNFSVersionFromMountOptions(mountOptions, "", supportedNFSVersions)
		if err != nil {
			return err
		}
		switch nfsVersion {
		case nfsVersion3:
			nfsV3Access = true
			protocolTypes = []string{gcnvapi.ProtocolTypeNFSv3}
		case nfsVersion4:
			fallthrough
		case nfsVersion41:
			nfsV41Access = true
			protocolTypes = []string{gcnvapi.ProtocolTypeNFSv41}
		case "":
			nfsV3Access = true
			nfsV41Access = true
			protocolTypes = []string{gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41}
		}

		apiExportRule = gcnvapi.ExportRule{
			AllowedClients: pool.InternalAttributes()[ExportRule],
			SMB:            smbAccess,
			Nfsv3:          nfsV3Access,
			Nfsv4:          nfsV41Access,
			RuleIndex:      1,
			AccessType:     gcnvapi.AccessTypeReadWrite,
		}

		exportPolicy = &gcnvapi.ExportPolicy{
			Rules: []gcnvapi.ExportRule{apiExportRule},
		}
	}

	// Set snapshot directory from volume config first (handles Docker case), then from pool
	// If none is set, set it based on mountOption by default; for nfsv3 => false, nfsv4/4.1 => true
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes()[SnapshotDir]
		// If snapshot directory is not set at pool level, then set default value based on mount option
		if snapshotDir == "" {
			if strings.HasPrefix(nfsVersion, "3") {
				snapshotDir = "false"
			} else {
				snapshotDir = "true"
			}
		}
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir; %v", err)
	}

	labels := d.getTelemetryLabels(ctx)
	for k, v := range pool.GetLabels(ctx, "") {
		if key, keyOK := d.fixGCPLabelKey(k); keyOK {
			labels[key] = d.fixGCPLabelValue(v)
		}
		if len(labels) > gcnvapi.MaxLabelCount {
			break
		}
	}

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(sizeBytes, 10) // requested size, not including reserve
	volConfig.ServiceLevel = serviceLevel
	volConfig.SnapshotDir = snapshotDir
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.UnixPermissions = unixPermissions

	// Find matching capacity pools
	cPools := d.API.CapacityPoolsForStoragePool(ctx, pool, serviceLevel)

	// Filter capacity pools based on requisite topology
	cPools = d.API.FilterCapacityPoolsOnTopology(ctx, cPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies)

	if len(cPools) == 0 {
		return fmt.Errorf("no GCNV storage pools found for Trident pool %s", pool.Name())
	}

	createErrors := multierr.Combine()

	// Try each capacity pool until one works
	for _, cPool := range cPools {

		if d.Config.NASType == sa.SMB {
			Logc(ctx).WithFields(LogFields{
				"capacityPool":    cPool.Name,
				"creationToken":   name,
				"size":            sizeWithReserveBytes,
				"serviceLevel":    serviceLevel,
				"snapshotDir":     snapshotDirBool,
				"snapshotReserve": snapshotReserve,
				"protocolTypes":   protocolTypes,
			}).Debug("Creating volume.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"capacityPool":    cPool.Name,
				"creationToken":   name,
				"size":            sizeWithReserveBytes,
				"unixPermissions": unixPermissions,
				"serviceLevel":    serviceLevel,
				"snapshotDir":     snapshotDirBool,
				"snapshotReserve": snapshotReserve,
				"protocolTypes":   protocolTypes,
				"exportPolicy":    fmt.Sprintf("%+v", exportPolicy),
			}).Debug("Creating volume.")
		}

		createRequest := &gcnvapi.VolumeCreateRequest{
			Name:              volConfig.Name,
			CreationToken:     name,
			CapacityPool:      cPool.Name,
			SizeBytes:         int64(sizeWithReserveBytes),
			ProtocolTypes:     protocolTypes,
			Labels:            labels,
			SnapshotReserve:   snapshotReservePtr,
			SnapshotDirectory: snapshotDirBool,
		}

		// Add unix permissions and export policy fields only to NFS volume
		if d.Config.NASType == sa.NFS {
			createRequest.UnixPermissions = unixPermissions
			createRequest.ExportPolicy = exportPolicy
			createRequest.SecurityStyle = gcnvapi.SecurityStyleUnix
		}

		// Create the volume
		volume, createErr := d.API.CreateVolume(ctx, createRequest)
		if createErr != nil {
			errMessage := fmt.Sprintf("GCNV pool %s; error creating volume %s: %v", cPool.Name, name, createErr)
			Logc(ctx).Error(errMessage)
			createErrors = multierr.Combine(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Always save the full GCP ID
		volConfig.InternalID = volume.FullName

		// Wait for creation to complete so that the mount targets are available
		return d.waitForVolumeCreate(ctx, volume)
	}

	return createErrors
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NASStorageDriver) CreateClone(
	ctx context.Context, sourceVolConfig, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":   "CreateClone",
		"Type":     "NASStorageDriver",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// ensure new volume doesn't exist, fail if so
	// get source volume, fail if nonexistent or if wrong region
	// if snapshot specified, read snapshots from source, fail if nonexistent
	// if no snap specified, create one, fail if error
	// create volume from snapshot

	// Make sure we got a valid name
	if err := d.validateVolumeName(cloneVolConfig.Name); err != nil {
		return err
	}

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(name); err != nil {
		return err
	}

	// Get the source volume
	sourceVolume, err := d.API.Volume(ctx, sourceVolConfig)
	if err != nil {
		return fmt.Errorf("could not find source volume; %v", err)
	}

	// If the volume already exists, bail out
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, cloneVolConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		if extantVolume.State == gcnvapi.VolumeStateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", gcnvapi.VolumeStateCreating, gcnvapi.VolumeStateReady))
		}
		return drivers.NewVolumeExistsError(name)
	}

	var sourceSnapshot *gcnvapi.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.API.SnapshotForVolume(ctx, sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot; %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.State != gcnvapi.SnapshotStateReady {
			return fmt.Errorf("source snapshot state is %s, it must be %s",
				sourceSnapshot.State, gcnvapi.SnapshotStateReady)
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapshot,
			"source":   sourceVolume.Name,
		}).Debug("Found source snapshot.")

	} else {

		// No source snapshot specified, so create one
		snapName := "snap-" + strings.ToLower(time.Now().UTC().Format(storage.SnapshotNameFormat))

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapName,
			"source":   sourceVolume.Name,
		}).Debug("Creating source snapshot.")

		sourceSnapshot, err = d.API.CreateSnapshot(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not create source snapshot; %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.API.WaitForSnapshotState(ctx, sourceSnapshot, sourceVolume, gcnvapi.SnapshotStateReady,
			[]string{gcnvapi.SnapshotStateError}, gcnvapi.SnapshotTimeout)
		if err != nil {
			return err
		}

		// Re-fetch the snapshot to populate the properties after create has completed
		sourceSnapshot, err = d.API.SnapshotForVolume(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not retrieve newly-created snapshot")
		}

		// Save the snapshot name in the volume config so we can auto-delete it later
		cloneVolConfig.CloneSourceSnapshotInternal = sourceSnapshot.Name

		Logc(ctx).WithFields(LogFields{
			"snapshot": sourceSnapshot.Name,
			"source":   sourceVolume.Name,
		}).Debug("Created source snapshot.")
	}

	// If RO clone is requested, don't create the volume on GCNV backend and return nil
	if cloneVolConfig.ReadOnlyClone {
		// Return error if snapshot directory is not enabled for RO clone
		if !sourceVolume.SnapshotDirectory {
			return fmt.Errorf("snapshot directory access is set to %t and readOnly clone is set to %t ",
				sourceVolume.SnapshotDirectory, cloneVolConfig.ReadOnlyClone)
		}
		return nil
	}

	var labels map[string]string
	labels = d.updateTelemetryLabels(ctx, sourceVolume)

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.Config.Labels),
		})
		for k, v := range storagePoolTemp.GetLabels(ctx, "") {
			if key, keyOK := d.fixGCPLabelKey(k); keyOK {
				labels[key] = d.fixGCPLabelValue(v)
			}
			if len(labels) > gcnvapi.MaxLabelCount {
				break
			}
		}
	}

	Logc(ctx).WithFields(LogFields{
		"creationToken":   name,
		"sourceVolume":    sourceVolume.CreationToken,
		"sourceSnapshot":  sourceSnapshot.Name,
		"unixPermissions": sourceVolume.UnixPermissions,
		"snapshotReserve": sourceVolume.SnapshotReserve,
	}).Debug("Cloning volume.")

	createRequest := &gcnvapi.VolumeCreateRequest{
		Name:              cloneVolConfig.Name,
		CreationToken:     name,
		CapacityPool:      sourceVolume.CapacityPool,
		SizeBytes:         sourceVolume.SizeBytes,
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		Labels:            labels,
		SnapshotReserve:   convert.ToPtr(sourceVolume.SnapshotReserve),
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SecurityStyle:     sourceVolume.SecurityStyle,
		SnapshotID:        sourceSnapshot.FullName,
	}

	// Add unix permissions and export policy fields only to NFS volume
	if d.Config.NASType == sa.NFS {
		createRequest.ExportPolicy = sourceVolume.ExportPolicy
		createRequest.UnixPermissions = sourceVolume.UnixPermissions
	}

	// Clone the volume
	clone, err := d.API.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Always save the full GCP ID
	cloneVolConfig.InternalID = clone.FullName

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, clone)
}

// Import finds an existing volume and makes it available for containers.  If ImportNotManaged is false, the
// volume is fully brought under Trident's management.
func (d *NASStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "NASStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.API.VolumeByNameOrID(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", originalName, err)
	}
	if volume.State != gcnvapi.VolumeStateReady {
		return fmt.Errorf("volume %s is in state %s and is not available", originalName, volume.State)
	}
	// Don't allow import for dual-protocol volume.
	// For dual-protocol volume the ProtocolTypes has two values [NFSv3, CIFS]
	if d.isDualProtocolVolume(volume) {
		return fmt.Errorf("trident doesn't support importing a dual-protocol volume '%s'", originalName)
	}

	// Ensure the volume may be imported by a capacity pool managed by this backend
	if err = d.API.EnsureVolumeInValidCapacityPool(ctx, volume); err != nil {
		return err
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.SizeBytes, 10)

	Logc(ctx).WithFields(LogFields{
		"creationToken": volume.CreationToken,
		"managed":       !volConfig.ImportNotManaged,
		"state":         volume.State,
		"capacityPool":  volume.CapacityPool,
		"sizeBytes":     volume.SizeBytes,
	}).Debug("Found volume to import.")

	var snapshotDirAccess bool
	// Modify the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if volConfig.SnapshotDir != "" {
			if snapshotDirAccess, err = strconv.ParseBool(volConfig.SnapshotDir); err != nil {
				return fmt.Errorf("could not import volume %s, snapshot directory access is set to %s",
					originalName, volConfig.SnapshotDir)
			}
		}

		// Update the volume labels
		labels := d.updateTelemetryLabels(ctx, volume)

		if d.Config.NASType == sa.SMB && volume.ProtocolTypes[0] == gcnvapi.ProtocolTypeSMB {
			if err = d.API.ModifyVolume(ctx, volume, labels, nil, &snapshotDirAccess, nil); err != nil {
				Logc(ctx).WithField("originalName", originalName).WithError(err).Error(
					"Could not import volume, volume modify failed.")
				return fmt.Errorf("could not import volume %s, volume modify failed; %v", originalName, err)
			}

			Logc(ctx).WithFields(LogFields{
				"name":          volume.Name,
				"creationToken": volume.CreationToken,
				"labels":        labels,
			}).Info("Volume modified.")

		} else if d.Config.NASType == sa.NFS && (volume.ProtocolTypes[0] == gcnvapi.ProtocolTypeNFSv3 || volume.
			ProtocolTypes[0] == gcnvapi.ProtocolTypeNFSv41) {
			// Update volume unix permissions.  Permissions specified in a PVC annotation take precedence
			// over the backend's unixPermissions config.
			unixPermissions := volConfig.UnixPermissions
			if unixPermissions == "" {
				unixPermissions = d.Config.UnixPermissions
			}
			if unixPermissions == "" {
				unixPermissions = volume.UnixPermissions
			}
			if unixPermissions != "" {
				if err = filesystem.ValidateOctalUnixPermissions(unixPermissions); err != nil {
					return fmt.Errorf("could not import volume %s; %v", originalName, err)
				}
			}

			err = d.API.ModifyVolume(ctx, volume, labels, &unixPermissions, &snapshotDirAccess, nil)
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).WithError(err).Error(
					"Could not import volume, volume modify failed.")
				return fmt.Errorf("could not import volume %s, volume modify failed; %v", originalName, err)
			}

			Logc(ctx).WithFields(LogFields{
				"name":            volume.Name,
				"creationToken":   volume.CreationToken,
				"labels":          labels,
				"unixPermissions": unixPermissions,
			}).Info("Volume modified.")
		} else {
			return fmt.Errorf("could not import volume '%s' due to backend and volume mismatch", originalName)
		}

		if _, err = d.API.WaitForVolumeState(
			ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError}, d.defaultTimeout()); err != nil {
			return fmt.Errorf("could not import volume %s; %v", originalName, err)
		}
	}

	// The GCNV creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = originalName

	// Always save the full GCP ID
	volConfig.InternalID = volume.FullName

	return nil
}

// Rename changes the name of a volume.  Not supported by this driver.
func (d *NASStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "NASStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	// Rename is only needed for the import workflow, and we aren't currently renaming the
	// GCNV volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

// getTelemetryLabels builds the standard telemetry labels that are set on each volume.
func (d *NASStorageDriver) getTelemetryLabels(_ context.Context) map[string]string {
	return map[string]string{
		"version":          d.fixGCPLabelValue(d.telemetry.TridentVersion),
		"backend_uuid":     d.fixGCPLabelValue(d.telemetry.TridentBackendUUID),
		"platform":         d.fixGCPLabelValue(d.telemetry.Platform),
		"platform_version": d.fixGCPLabelValue(d.telemetry.PlatformVersion),
		"plugin":           d.fixGCPLabelValue(d.telemetry.Plugin),
	}
}

// updateTelemetryLabels updates a volume's labels to include the standard telemetry labels.
func (d *NASStorageDriver) updateTelemetryLabels(ctx context.Context, volume *gcnvapi.Volume) map[string]string {
	if volume.Labels == nil {
		volume.Labels = make(map[string]string)
	}

	newLabels := volume.Labels
	for k, v := range d.getTelemetryLabels(ctx) {
		newLabels[k] = v
	}
	return newLabels
}

// fixGCPLabelKey accepts a label key and modifies it to satisfy GCP label key rules, or returns
// false if not possible.
func (d *NASStorageDriver) fixGCPLabelKey(s string) (string, bool) {
	// Check if the string is empty
	if len(s) == 0 {
		return "", false
	}

	// Convert the string to lowercase
	s = strings.ToLower(s)

	// Replace all disallowed characters with underscores
	s = gcpLabelRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("_", len(m))
	})

	// Check if the first character is a lowercase letter
	if !unicode.IsLower(rune(s[0])) {
		return "", false
	}

	// Shorten the string to a maximum of 63 characters
	s = convert.TruncateString(s, gcnvapi.MaxLabelLength)

	return s, true
}

// fixGCPLabelValue accepts a label value and modifies it to satisfy GCP label value rules.
func (d *NASStorageDriver) fixGCPLabelValue(s string) string {
	// Convert the string to lowercase
	s = strings.ToLower(s)

	// Replace all disallowed characters with underscores
	s = gcpLabelRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("_", len(m))
	})

	// Shorten the string to a maximum of 63 characters
	s = convert.TruncateString(s, gcnvapi.MaxLabelLength)

	return s
}

// waitForVolumeCreate waits for volume creation to complete by reaching the Available state.  If the
// volume reaches a terminal state (Error), the volume is deleted.  If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
func (d *NASStorageDriver) waitForVolumeCreate(ctx context.Context, volume *gcnvapi.Volume) error {
	state, err := d.API.WaitForVolumeState(
		ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError}, d.volumeCreateTimeout)
	if err != nil {

		logFields := LogFields{"volume": volume.Name}

		switch state {

		case gcnvapi.VolumeStateUnspecified, gcnvapi.VolumeStateCreating:
			Logc(ctx).WithFields(logFields).Debugf("Volume is in %s state.", state)
			return errors.VolumeCreatingError(err.Error())

		case gcnvapi.VolumeStateDeleting:
			// Wait for deletion to complete
			_, errDelete := d.API.WaitForVolumeState(
				ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError}, d.defaultTimeout())
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
			}
			return errDelete

		case gcnvapi.VolumeStateError:
			// Delete a failed volume
			errDelete := d.API.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
				return errDelete
			} else {
				Logc(ctx).WithField("volume", volume.Name).Info("Volume deleted.")
			}

		default:
			Logc(ctx).WithFields(logFields).Errorf("unexpected volume state %s found for volume", state)
		}
	}

	return nil
}

// Destroy deletes a volume.
func (d *NASStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) (err error) {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NASStorageDriver",
		"name":   name,
	}

	// If it's a RO clone, no need to delete the volume
	if volConfig.ReadOnlyClone {
		return nil
	}

	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Update resource cache as needed
	if err = d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// If this volume was cloned from an automatic snapshot, delete the snapshot after deleting the volume.
	defer func() {
		d.deleteAutomaticSnapshot(ctx, err, volConfig)
	}()

	// If volume doesn't exist, return success
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.State == gcnvapi.VolumeStateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.API.WaitForVolumeState(ctx, extantVolume, gcnvapi.VolumeStateDeleted,
			[]string{gcnvapi.VolumeStateError}, d.volumeCreateTimeout)
		return err
	}

	// Delete the volume
	if err = d.API.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	Logc(ctx).WithField("volume", extantVolume.Name).Info("Volume deleted.")

	// Wait for deletion to complete
	_, err = d.API.WaitForVolumeState(ctx, extantVolume, gcnvapi.VolumeStateDeleted,
		[]string{gcnvapi.VolumeStateError}, d.defaultTimeout())
	return err
}

// deleteAutomaticSnapshot deletes a snapshot that was created automatically during volume clone creation.
// An automatic snapshot is detected by the presence of CloneSourceSnapshotInternal in the volume config
// while CloneSourceSnapshot is not set.  This method is called after the volume has been deleted, and it
// will only attempt snapshot deletion if the clone volume deletion completed without error.  This is a
// best-effort method, and any errors encountered will be logged but not returned.
func (d *NASStorageDriver) deleteAutomaticSnapshot(
	ctx context.Context, volDeleteError error, volConfig *storage.VolumeConfig,
) {
	snapshot := volConfig.CloneSourceSnapshotInternal
	cloneSourceName := volConfig.CloneSourceVolumeInternal
	cloneName := volConfig.InternalName
	fields := LogFields{
		"Method":          "DeleteSnapshot",
		"Type":            "NASStorageDriver",
		"snapshotName":    snapshot,
		"cloneSourceName": cloneSourceName,
		"cloneName":       cloneName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> deleteAutomaticSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< deleteAutomaticSnapshot")

	logFields := LogFields{
		"snapshotName":    snapshot,
		"cloneSourceName": cloneSourceName,
		"cloneName":       cloneName,
	}

	// Check if there is anything to do
	if !(volConfig.CloneSourceSnapshot == "" && volConfig.CloneSourceSnapshotInternal != "") {
		Logc(ctx).WithFields(logFields).Debug("No automatic clone source snapshot existed, skipping cleanup.")
		return
	}

	// If the clone volume couldn't be deleted, don't attempt to delete any automatic snapshot.
	if volDeleteError != nil {
		Logc(ctx).WithFields(logFields).Debug("Error deleting volume, skipping automatic snapshot cleanup.")
		return
	}

	// Get the volume
	sourceVolume, err := d.API.VolumeByName(ctx, cloneSourceName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Volume for automatic snapshot not found, skipping cleanup.")
		} else {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Error checking for automatic snapshot volume. " +
				"Any automatic snapshot must be manually deleted.")
		}
		return
	}

	// Get the snapshot
	sourceSnapshot, err := d.API.SnapshotForVolume(ctx, sourceVolume, snapshot)
	if err != nil {
		// If the snapshot is already gone, return success
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Automatic snapshot not found, skipping cleanup.")
		} else {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Error checking for automatic snapshot. " +
				"Any automatic snapshot must be manually deleted.")
		}
		return
	}

	// Delete the snapshot
	if err = d.API.DeleteSnapshot(ctx, sourceVolume, sourceSnapshot); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Errorf("Automatic snapshot could not be " +
			"cleaned up and must be manually deleted.")
	}

	return
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	var volume *gcnvapi.Volume
	var err error

	name := volConfig.InternalName
	fields := LogFields{
		"Method": "Publish",
		"Type":   "NASStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// If it's a RO clone, get source volume to populate publish info
	if volConfig.ReadOnlyClone {
		volume, err = d.API.VolumeByName(ctx, volConfig.CloneSourceVolumeInternal)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	} else {
		// Get the volume
		volume, err = d.API.Volume(ctx, volConfig)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NFSMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add required fields for attaching SMB volume
	if d.Config.NASType == sa.SMB {
		server, share, exportErr := d.parseSMBExport(volume.MountTargets[0].ExportPath)
		if exportErr != nil {
			return exportErr
		}

		publishInfo.SMBPath = "\\" + share
		publishInfo.SMBServer = server
		publishInfo.FilesystemType = sa.SMB
	} else {
		protocol := ""
		nfsVersion, versionErr := nfs.GetNFSVersionFromMountOptions(mountOptions, "", supportedNFSVersions)
		if versionErr != nil {
			return versionErr
		}
		switch nfsVersion {
		case nfsVersion3:
			protocol = gcnvapi.ProtocolTypeNFSv3
		case nfsVersion4:
			fallthrough
		case nfsVersion41:
			protocol = gcnvapi.ProtocolTypeNFSv41
		default:
			// No preference, use first listed NFS mount target
		}

		server, share, exportErr := d.nfsExportComponentsForProtocol(volume, protocol)
		if exportErr != nil {
			return exportErr
		}

		// Add fields needed by Attach
		publishInfo.NfsPath = "/" + share
		publishInfo.NfsServerIP = server
		publishInfo.FilesystemType = sa.NFS
		publishInfo.MountOptions = mountOptions
	}

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NASStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		// The GCNV volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.

		Logc(ctx).WithFields(LogFields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}).Debug("Volume for snapshot not found.")

		return nil, nil
	}

	// Get the snapshot
	snapshot, err := d.API.SnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}

	if snapshot.State != gcnvapi.SnapshotStateReady {
		return nil, fmt.Errorf("snapshot %s state is %s", internalSnapName, snapshot.State)
	}

	created := snapshot.Created.UTC().Format(convert.TimestampFormat)

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      created,
	}).Debug("Found snapshot.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   created,
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume.
func (d *NASStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NASStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s; %v", internalVolName, err)
	}

	snapshots, err := d.API.SnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, snapshot := range *snapshots {

		// Filter out snapshots in an unavailable state
		if snapshot.State != gcnvapi.SnapshotStateReady {
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
			Created:   snapshot.Created.UTC().Format(convert.TimestampFormat),
			SizeBytes: 0,
			State:     storage.SnapshotStateOnline,
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *NASStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Check if volume exists
	volumeExists, sourceVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Create the snapshot
	snapshot, err := d.API.CreateSnapshot(ctx, sourceVolume, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot; %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.API.WaitForSnapshotState(
		ctx, snapshot, sourceVolume, gcnvapi.SnapshotStateReady, []string{gcnvapi.SnapshotStateError}, gcnvapi.SnapshotTimeout)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Info("Snapshot created.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.UTC().Format(convert.TimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", internalVolName, err)
	}

	// Get the snapshot
	snapshot, err := d.API.SnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	// Do the restore
	if err = d.API.RestoreSnapshot(ctx, volume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	_, err = d.API.WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady,
		[]string{gcnvapi.VolumeStateError, gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateDeleted}, gcnvapi.DefaultSDKTimeout,
	)
	return err
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *NASStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		// The GCNV volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.

		Logc(ctx).WithFields(LogFields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}).Debug("Volume for snapshot not found.")

		return nil
	}

	snapshot, err := d.API.SnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		// If the snapshot is already gone, return success
		if errors.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("unable to find snapshot %s; %v", internalSnapName, err)
	}

	if err = d.API.DeleteSnapshot(ctx, extantVolume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	return d.API.WaitForSnapshotState(
		ctx, snapshot, extantVolume, gcnvapi.SnapshotStateDeleted, []string{gcnvapi.SnapshotStateError}, gcnvapi.SnapshotTimeout,
	)
}

// List returns the list of volumes associated with this backend.
func (d *NASStorageDriver) List(ctx context.Context) ([]string, error) {
	fields := LogFields{"Method": "List", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> List")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< List")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	volumes, err := d.API.Volumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	volumeNames := make([]string, 0)

	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.State {
		case gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateError, gcnvapi.VolumeStateDisabled:
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

// Get tests for the existence of a volume.
func (d *NASStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	if _, err := d.API.VolumeByName(ctx, name); err != nil {
		return fmt.Errorf("could not get volume %s; %v", name, err)
	}

	return nil
}

// Resize increases a volume's quota.
func (d *NASStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":    "Resize",
		"Type":      "NASStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// If the volume state isn't Available, return an error
	if volume.State != gcnvapi.VolumeStateReady {
		return fmt.Errorf("volume %s state is %s, not %s", name, volume.State, gcnvapi.VolumeStateReady)
	}

	// Include the snapshot reserve in the new size
	if volume.SnapshotReserve > math.MaxInt {
		return fmt.Errorf("snapshot reserve too large")
	}
	sizeWithReserveBytes := drivers.CalculateVolumeSizeBytes(ctx, name, sizeBytes, int(volume.SnapshotReserve))

	// If the volume is already the requested size, there's nothing to do
	if int64(sizeWithReserveBytes) == volume.SizeBytes {
		volConfigSize := strconv.FormatUint(sizeBytes, 10)
		if volConfigSize != volConfig.Size {
			volConfig.Size = volConfigSize
		}
		return nil
	}

	// Make sure we're not shrinking the volume
	if int64(sizeWithReserveBytes) < volume.SizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d",
			sizeWithReserveBytes, volume.SizeBytes)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	_, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeWithReserveBytes, d.Config.CommonStorageDriverConfig)
	if err != nil {
		return err
	}

	// Resize the volume
	if err = d.API.ResizeVolume(ctx, volume, int64(sizeWithReserveBytes)); err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10) // requested size, not including reserve
	return nil
}

// GetStorageBackendSpecs retrieves storage capabilities and register pools with specified backend.
func (d *NASStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	backend.SetName(d.BackendName())

	for _, pool := range d.pools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	return nil
}

// CreatePrepare is called prior to volume creation.  Currently its only role is to create the internal volume name.
func (d *NASStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig, pool)
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return []string{}
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *NASStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.GCNVNASStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NASStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. Project number
	// 2. Location
	// 3. Storage pool - contains at least one pool depending on the backend configuration.
	cPools := d.API.CapacityPoolsForStoragePools(ctx)
	backendPools := make([]drivers.GCNVNASStorageBackendPool, 0, len(cPools))
	for _, cPool := range cPools {
		backendPool := drivers.GCNVNASStorageBackendPool{
			ProjectNumber: d.Config.ProjectNumber,
			Location:      cPool.Location,
			StoragePool:   cPool.Name,
		}
		backendPools = append(backendPools, backendPool)
	}
	return backendPools
}

// GetInternalVolumeName accepts the name of a volume being created and returns what the internal name
// should be, depending on backend requirements and Trident's operating context.
func (d *NASStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + volConfig.Name
	} else if csiRegex.MatchString(volConfig.Name) {
		// If the name is from CSI (i.e. contains a UUID), just use it as-is
		Logc(ctx).WithField("volumeInternal", volConfig.Name).Debug("Using volume name as internal name.")
		return volConfig.Name
	} else {
		// Cloud volumes have strict limits on volume mount paths, so for cloud
		// infrastructure like Trident, the simplest approach is to generate a
		// UUID-based name with a prefix that won't exceed the length limit.
		return "gcnv-" + uuid.NewString()
	}
}

// CreateFollowup is called after volume creation and sets the access info in the volume config.
func (d *NASStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	var volume *gcnvapi.Volume
	var err error

	name := volConfig.InternalName
	fields := LogFields{
		"Method": "CreateFollowup",
		"Type":   "NASStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	// If it's a RO clone, get source volume to populate access details
	if volConfig.ReadOnlyClone {
		volume, err = d.API.VolumeByName(ctx, volConfig.CloneSourceVolumeInternal)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	} else {
		// Get the volume
		volume, err = d.API.Volume(ctx, volConfig)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	}

	// Ensure volume is in a good state
	if volume.State != gcnvapi.VolumeStateReady {
		return fmt.Errorf("volume %s is in %s state, not %s", name, volume.State, gcnvapi.VolumeStateReady)
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volConfig.InternalName)
	}

	// Set the mount target based on the NASType
	if d.Config.NASType == sa.SMB {
		volConfig.AccessInfo.SMBPath = constructVolumeAccessPath(volConfig, volume, sa.SMB)
		volConfig.AccessInfo.SMBServer = (volume.MountTargets)[0].ExportPath
		volConfig.FileSystem = sa.SMB
	} else {
		server, share, exportErr := d.nfsExportComponentsForProtocol(volume, "")
		if exportErr != nil {
			return exportErr
		}

		volConfig.AccessInfo.NfsPath = "/" + share
		volConfig.AccessInfo.NfsServerIP = server
		volConfig.FileSystem = sa.NFS
	}

	return nil
}

// GetProtocol returns the protocol supported by this driver (File).
func (d *NASStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

// StoreConfig adds this backend's config to the persistent config struct, as needed by Trident's persistence layer.
func (d *NASStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.GCNVConfig = &d.Config
}

// GetExternalConfig returns a clone of this backend's config, sanitized for external consumption.
func (d *NASStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.GCNVNASStorageDriverConfig
	drivers.Clone(ctx, d.Config, &cloneConfig)

	cloneConfig.APIKey = drivers.GCPPrivateKey{
		Type:                    tridentconfig.REDACTED,
		ProjectID:               tridentconfig.REDACTED,
		PrivateKeyID:            tridentconfig.REDACTED,
		PrivateKey:              tridentconfig.REDACTED,
		ClientEmail:             tridentconfig.REDACTED,
		ClientID:                tridentconfig.REDACTED,
		AuthURI:                 tridentconfig.REDACTED,
		TokenURI:                tridentconfig.REDACTED,
		AuthProviderX509CertURL: tridentconfig.REDACTED,
		ClientX509CertURL:       tridentconfig.REDACTED,
	}
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: tridentconfig.REDACTED,
		drivers.KeyType: tridentconfig.REDACTED,
	} // redact the credentials

	return cloneConfig
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name used when
// creating the volume.
func (d *NASStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update GCNV resource cache; %v", err)
	}

	filesystem, err := d.API.VolumeByNameOrID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(filesystem), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	fields := LogFields{"Method": "GetVolumeExternalWrappers", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetVolumeExternalWrappers")
	defer Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetVolumeExternalWrappers")

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all volumes
	volumes, err := d.API.Volumes(ctx)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	prefix := *d.Config.StoragePrefix

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.State {
		case gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateError, gcnvapi.VolumeStateDisabled:
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
func (d *NASStorageDriver) getVolumeExternal(volumeAttrs *gcnvapi.Volume) *storage.VolumeExternal {
	internalName := volumeAttrs.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    volumeAttrs.CreationToken,
		Size:            strconv.FormatInt(volumeAttrs.SizeBytes, 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     strconv.FormatBool(volumeAttrs.SnapshotDirectory),
		UnixPermissions: volumeAttrs.UnixPermissions,
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      models.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
		ServiceLevel:    volumeAttrs.ServiceLevel,
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}

// String implements stringer interface for the NASStorageDriver driver.
func (d *NASStorageDriver) String() string {
	return convert.ToStringRedacted(d, []string{"SDK"}, d.GetExternalConfig(context.Background()))
}

// GoString implements GoStringer interface for the NASStorageDriver driver.
func (d *NASStorageDriver) GoString() string {
	return d.String()
}

// GetUpdateType returns a bitmap populated with updates to the driver.
func (d *NASStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
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

// ReconcileNodeAccess updates a per-backend export policy to match the set of Kubernetes cluster
// nodes.  Not supported by this driver.
func (d *NASStorageDriver) ReconcileNodeAccess(
	ctx context.Context, _ []*models.Node, _, _ string,
) error {
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return nil
}

func (d *NASStorageDriver) ReconcileVolumeNodeAccess(ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// validateGCNVStoragePrefix ensures the storage prefix is valid
func validateGCNVStoragePrefix(storagePrefix string) error {
	if !storagePrefixRegex.MatchString(storagePrefix) {
		return fmt.Errorf("storage prefix may only contain letters and hyphens and must begin with a letter")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d *NASStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d *NASStorageDriver) nfsExportComponentsForProtocol(
	volume *gcnvapi.Volume, protocol string,
) (server, share string, err error) {
	switch protocol {
	case gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41, "":
		// First find matching protocol
		for _, mountTarget := range volume.MountTargets {
			if mountTarget.Protocol == protocol {
				return d.parseNFSExport(mountTarget.ExportPath)
			}
		}
		// Fall back to any NFS mount
		for _, mountTarget := range volume.MountTargets {
			if collection.ContainsString(
				[]string{gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41}, mountTarget.Protocol) {
				return d.parseNFSExport(mountTarget.ExportPath)
			}
		}
		return "", "", fmt.Errorf("no NFS mount target found on volume %s", volume.Name)
	default:
		return "", "", fmt.Errorf("invalid NFS protocol (%s)", protocol)
	}
}

func (d *NASStorageDriver) parseNFSExport(export string) (server, share string, err error) {
	match := nfsMountPathRegex.FindStringSubmatch(export)

	if match == nil {
		err = fmt.Errorf("NFS export path %s is invalid", export)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range nfsMountPathRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	server = paramsMap["server"]
	share = paramsMap["share"]

	return
}

func (d *NASStorageDriver) parseSMBExport(export string) (server, share string, err error) {
	match := smbMountPathRegex.FindStringSubmatch(export)

	if match == nil {
		err = fmt.Errorf("SMB export path %s is invalid", export)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range smbMountPathRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	server = paramsMap["server"]
	share = paramsMap["share"]

	return
}

func (d *NASStorageDriver) isDualProtocolVolume(volume *gcnvapi.Volume) bool {
	var nfs, smb bool

	for _, protocol := range volume.ProtocolTypes {
		switch protocol {
		case gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41:
			nfs = true
		case gcnvapi.ProtocolTypeSMB:
			smb = true
		}
	}
	return nfs && smb
}

func constructVolumeAccessPath(
	volConfig *storage.VolumeConfig, volume *gcnvapi.Volume, protocol string,
) string {
	switch protocol {
	case sa.NFS:
		if volConfig.ReadOnlyClone {
			return "/" + volConfig.CloneSourceVolumeInternal + "/" + ".snapshot" + "/" + volConfig.CloneSourceSnapshot
		}
		return "/" + volume.CreationToken
	case sa.SMB:
		if volConfig.ReadOnlyClone {
			return "\\" + volConfig.CloneSourceVolumeInternal + "\\" + "~snapshot" + "\\" + volConfig.CloneSourceSnapshot
		}
		return "\\" + volume.CreationToken
	}
	return ""
}
