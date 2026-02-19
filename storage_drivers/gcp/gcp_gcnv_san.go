// Copyright 2025 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
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
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	TridentLabelFSType        = "trident.netapp.io/fstype"
	TridentLabelFormatOptions = "trident.netapp.io/formatOptions"

	// Internal attribute keys for storage pools (SAN-specific)
	FileSystemType = "fileSystemType"
	FormatOptions  = "formatOptions"

	// Default format options for filesystems (matching ONTAP SAN defaults)
	DefaultExt3FormatOptions = ""
	DefaultExt4FormatOptions = ""
	DefaultXfsFormatOptions  = ""
)

// newGCNVDriver is injected for unit testing to avoid needing real GCP credentials.
// Production code uses api.NewDriver.
var newGCNVDriver = api.NewDriver

var (
	// sanVolumeNameRegex validates the user-visible volume name (1-63 chars, lowercase alphanumeric, start with letter)
	sanVolumeNameRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	// sanVolumeCreationTokenRegex validates the internal volume name (1-80 chars, lowercase alphanumeric, start with letter)
	sanVolumeCreationTokenRegex = regexp.MustCompile(`^[a-z]([a-z0-9_]{0,78}[a-z0-9])?$`)
)

// MaxGCNVVolumeSizeBytes is the maximum volume size supported by GCNV for iSCSI volumes
// (128 TiB as per GCNV documentation for Flex service level with Unified storage pools).
const MaxGCNVVolumeSizeBytes = int64(128) * 1024 * api.GiBBytes // 128 TiB

// calculateActualSizeBytes computes the actual size to request from GCNV to ensure the
// kernel-visible LUN usable space is greater than or equal to what the user requested.
//
// GCNV block volumes exhibit a small discrepancy between the requested capacity and the
// actual kernel-visible device size due to internal metadata overhead. Testing shows:
//   - On initial creation: ~300 KiB less than requested
//   - On resize: potentially up to ~107 MiB less than requested
//
// To guarantee users always get at least their requested usable space, we:
//  1. Round up to the next whole GiB (GCNV only accepts whole GiB values, and integer
//     division in the API layer would otherwise truncate/round down)
//  2. Add 1 GiB buffer to absorb the metadata overhead
//
// Example: User requests 2.5 GiB → rounds up to 3 GiB → adds 1 GiB → requests 4 GiB from GCNV
// Result: User gets ~3.999+ GiB usable space, well above their 2.5 GiB request
//
// Returns an error if the calculated size would exceed GCNV's 128 TiB maximum volume size.
func calculateActualSizeBytes(requestedBytes int64) (int64, error) {
	// GCNV supports volumes up to 128 TiB. Since we add up to 2 GiB (rounding + buffer),
	// the maximum safe request is 128 TiB - 2 GiB.
	maxSafeRequest := MaxGCNVVolumeSizeBytes - 2*api.GiBBytes
	if requestedBytes > maxSafeRequest {
		return 0, fmt.Errorf("requested size %d bytes exceeds GCNV maximum; "+
			"maximum supported request is %d bytes (128 TiB - 2 GiB buffer)",
			requestedBytes, maxSafeRequest)
	}

	// Round up to next whole GiB (ceiling division)
	requestedGiB := (requestedBytes + api.GiBBytes - 1) / api.GiBBytes

	// Add 1 GiB buffer to ensure usable space exceeds requested size
	actualGiB := requestedGiB + 1

	return actualGiB * api.GiBBytes, nil
}

// SANStorageDriver is for iSCSI storage provisioning using Google Cloud NetApp Volumes
type SANStorageDriver struct {
	initialized         bool
	Config              drivers.GCNVStorageDriverConfig
	API                 api.GCNV
	pools               map[string]storage.Pool
	tridentUUID         string
	telemetry           *Telemetry
	lunMutex            *locks.GCNamedMutex
	hostGroupMutex      *locks.GCNamedMutex
	volumeCreateTimeout time.Duration
}

// Name returns the name of this driver.
func (d *SANStorageDriver) Name() string {
	return tridentconfig.GCNVSANStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance.
func (d *SANStorageDriver) defaultBackendName() string {
	id := crypto.RandomString(5)
	if len(d.Config.APIKey.PrivateKeyID) > 5 {
		id = d.Config.APIKey.PrivateKeyID[0:5]
	}
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), id)
}

// BackendName returns the name of the backend managed by this driver instance.
func (d *SANStorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	}
	return d.defaultBackendName()
}

// validateVolumeName checks that the name of a new volume matches the requirements of a GCNV volume name.
func (d *SANStorageDriver) validateVolumeName(name string) error {
	if !sanVolumeNameRegex.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be 1-63 characters long, "+
			"begin with a letter, and contain only lowercase letters, numbers, or hyphens", name)
	}
	return nil
}

// validateCreationToken checks that the creation token of a new volume matches the requirements of a creation token.
func (d *SANStorageDriver) validateCreationToken(name string) error {
	if !sanVolumeCreationTokenRegex.MatchString(name) {
		return fmt.Errorf("volume internal name '%s' is not allowed; it must be 1-80 characters long, "+
			"begin with a letter, and contain only lowercase letters, numbers, or underscores", name)
	}
	return nil
}

// GetConfig returns the config of this driver.
func (d *SANStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

// defaultCreateTimeout sets the driver timeout for volume create/delete operations.
// Note: SAN driver only supports CSI context (Docker is rejected in Initialize).
func (d *SANStorageDriver) defaultCreateTimeout() time.Duration {
	return api.VolumeCreateTimeout
}

// defaultTimeout sets the driver timeout for most API operations.
// Note: SAN driver only supports CSI context (Docker is rejected in Initialize).
func (d *SANStorageDriver) defaultTimeout() time.Duration {
	return api.DefaultTimeout
}

// Initialize initializes this driver from the provided config.
func (d *SANStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "SANStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	// Validate driver context
	if driverContext != tridentconfig.ContextCSI {
		return fmt.Errorf("invalid driver context: %s", driverContext)
	}

	commonConfig.DriverContext = driverContext

	// Initialize garbage-collected mutexes
	d.lunMutex = locks.NewGCNamedMutex()
	d.hostGroupMutex = locks.NewGCNamedMutex()

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

	// Store UUID for host group naming
	d.tridentUUID = backendUUID

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = d.initializeGCNVAPIClient(ctx, &d.Config); err != nil {
			return fmt.Errorf("error validating %s GCNV API. %v", d.Name(), err)
		}
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %v", err)
	}
	d.Config.BackendPools = pools

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":       *d.Config.StoragePrefix,
		"Size":                d.Config.Size,
		"ServiceLevel":        d.Config.ServiceLevel,
		"LimitVolumeSize":     d.Config.LimitVolumeSize,
		"VolumeCreateTimeout": d.volumeCreateTimeout,
		"FileSystemType":      d.Config.FileSystemType,
		"FormatOptions":       d.Config.FormatOptions,
	}).Debug("GCNV SAN driver initialized")

	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated).
func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded.
func (d *SANStorageDriver) Terminate(ctx context.Context, backendUUID string) {
	fields := LogFields{"Method": "Terminate", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	d.initialized = false
}

// Helper methods

// initializeGCNVConfig parses the GCNV config, mixing in the specified common config.
func (d *SANStorageDriver) initializeGCNVConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.GCNVStorageDriverConfig, error) {
	fields := LogFields{"Method": "initializeGCNVConfig", "Type": "SANStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> initializeGCNVConfig")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< initializeGCNVConfig")

	config := &drivers.GCNVStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into GCNVStorageDriverConfig object
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
func (d *SANStorageDriver) initializeGCNVAPIClient(
	ctx context.Context, config *drivers.GCNVStorageDriverConfig,
) (api.GCNV, error) {
	fields := LogFields{"Method": "initializeGCNVAPIClient", "Type": "SANStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> initializeGCNVAPIClient")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< initializeGCNVAPIClient")

	sdkTimeout := api.DefaultSDKTimeout
	if config.SDKTimeout != "" {
		if i, parseErr := strconv.ParseInt(d.Config.SDKTimeout, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.SDKTimeout).WithError(parseErr).Error(
				"Invalid value for SDK timeout.")
			return nil, parseErr
		} else {
			sdkTimeout = time.Duration(i) * time.Second
		}
	}

	maxCacheAge := api.DefaultMaxCacheAge
	if config.MaxCacheAge != "" {
		if i, parseErr := strconv.ParseInt(d.Config.MaxCacheAge, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.MaxCacheAge).WithError(parseErr).Error(
				"Invalid value for max cache age.")
			return nil, parseErr
		} else {
			maxCacheAge = time.Duration(i) * time.Second
		}
	}

	gcnv, err := newGCNVDriver(ctx, &api.ClientConfig{
		ProjectNumber:       config.ProjectNumber,
		Location:            config.Location,
		APIKey:              &config.APIKey,
		APIEndpoint:         config.APIEndpoint,
		WIPCredentialConfig: config.WIPCredentialConfig,
		DebugTraceFlags:     config.DebugTraceFlags,
		SDKTimeout:          sdkTimeout,
		MaxCacheAge:         maxCacheAge,
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

func (d *SANStorageDriver) initializeStoragePools(ctx context.Context) {
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

		if d.Config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes()[Size] = d.Config.Size
		pool.InternalAttributes()[ServiceLevel] = convert.ToTitle(d.Config.ServiceLevel)
		pool.InternalAttributes()[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes()[Network] = d.Config.Network
		pool.InternalAttributes()[CapacityPools] = strings.Join(d.Config.StoragePools, ",")
		pool.InternalAttributes()[FileSystemType] = d.Config.FileSystemType
		pool.InternalAttributes()[FormatOptions] = d.Config.FormatOptions

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

			capacityPools := d.Config.StoragePools
			if vpool.StoragePools != nil {
				capacityPools = vpool.StoragePools
			}

			serviceLevel := d.Config.ServiceLevel
			if vpool.ServiceLevel != "" {
				serviceLevel = vpool.ServiceLevel
			}

			network := d.Config.Network
			if vpool.Network != "" {
				network = vpool.Network
			}

			fileSystemType := d.Config.FileSystemType
			if vpool.FileSystemType != "" {
				fileSystemType = vpool.FileSystemType
			}

			formatOptions := d.Config.FormatOptions
			if vpool.FormatOptions != "" {
				formatOptions = vpool.FormatOptions
			}

			snapshotReserve := d.Config.SnapshotReserve
			if vpool.SnapshotReserve != "" {
				snapshotReserve = vpool.SnapshotReserve
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)

			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[ServiceLevel] = convert.ToTitle(serviceLevel)
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[Network] = network
			pool.InternalAttributes()[CapacityPools] = strings.Join(capacityPools, ",")
			pool.InternalAttributes()[FileSystemType] = fileSystemType
			pool.InternalAttributes()[FormatOptions] = formatOptions

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config.
func (d *SANStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.GCNVStorageDriverConfig,
) error {
	fields := LogFields{"Method": "populateConfigurationDefaults", "Type": "SANStorageDriver"}
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
		config.ServiceLevel = api.ServiceLevelFlex
	}

	if config.SnapshotReserve == "" {
		config.SnapshotReserve = defaultSnapshotReserve
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	// VolumeCreateTimeout
	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		i, err := strconv.ParseInt(config.VolumeCreateTimeout, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", config.VolumeCreateTimeout).Errorf(
				"Invalid volume create timeout period. %v", err)
			return err
		}
		volumeCreateTimeout = time.Duration(i) * time.Second
	}
	d.volumeCreateTimeout = volumeCreateTimeout

	// FileSystemType defaults to ext4
	if config.FileSystemType == "" {
		config.FileSystemType = drivers.DefaultFileSystemType
	}

	// FormatOptions defaults based on filesystem type
	if config.FormatOptions == "" {
		switch config.FileSystemType {
		case "ext3":
			config.FormatOptions = DefaultExt3FormatOptions
		case "ext4":
			config.FormatOptions = DefaultExt4FormatOptions
		case "xfs":
			config.FormatOptions = DefaultXfsFormatOptions
		}
	}

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":       *config.StoragePrefix,
		"Size":                config.Size,
		"ServiceLevel":        config.ServiceLevel,
		"SnapshotReserve":     config.SnapshotReserve,
		"LimitVolumeSize":     config.LimitVolumeSize,
		"VolumeCreateTimeout": d.volumeCreateTimeout,
		"FileSystemType":      config.FileSystemType,
		"FormatOptions":       config.FormatOptions,
	}).Debug("Configuration defaults")

	return nil
}

// initializeTelemetry initializes the telemetry struct with backend information.
func (d *SANStorageDriver) initializeTelemetry(_ context.Context, backendUUID string) {
	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
	}
}

// validate ensures the driver configuration and execution environment are valid and working.
func (d *SANStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	// Ensure storage prefix is compatible with cloud service
	if err := validateGCNVStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {
		// Validate service level - SAN driver requires Flex service level for unified storage pools
		serviceLevel := pool.InternalAttributes()[ServiceLevel]
		switch serviceLevel {
		case api.ServiceLevelFlex, "":
			// Flex is required for SAN (iSCSI) volumes; empty defaults to Flex
			break
		default:
			return fmt.Errorf("invalid service level in pool %s: %s; SAN driver requires Flex service level",
				poolName, pool.InternalAttributes()[ServiceLevel])
		}

		// Validate default size
		if _, err := capacity.ToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s; %v", poolName, err)
		}

		// Validate filesystem type
		fstype := pool.InternalAttributes()[FileSystemType]
		switch fstype {
		case "ext3", "ext4", "xfs", "":
			break
		default:
			return fmt.Errorf("invalid filesystem type in pool %s: %s", poolName, fstype)
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
	}

	return nil
}

// getStorageBackendPools returns the storage backend pools for this driver.
func (d *SANStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.GCNVStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "SANStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. Project number
	// 2. Location
	// 3. Storage pool - contains at least one pool depending on the backend configuration.
	cPools := d.API.CapacityPoolsForStoragePools(ctx)
	backendPools := make([]drivers.GCNVStorageBackendPool, 0, len(cPools))
	for _, cPool := range cPools {
		backendPool := drivers.GCNVStorageBackendPool{
			ProjectNumber: d.Config.ProjectNumber,
			Location:      cPool.Location,
			StoragePool:   cPool.Name,
		}
		backendPools = append(backendPools, backendPool)
	}
	return backendPools
}

// poolName constructs the name of the pool reported by this driver instance.
func (d *SANStorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// getNodeSpecificHostGroupName generates a valid GCNV host group ID that:
// - Starts with a lowercase letter
// - Contains only lowercase letters, numbers, and hyphens
// - Does not end with a hyphen
// - Is at most 63 characters long (GCNV requirement: 1 letter + up to 62 alphanumeric/hyphens)
// - Is unique per node and backend
//
// Note: This differs from ONTAP's getNodeSpecificIgroupName which simply truncates the node name
// when the igroup name exceeds 96 chars. GCNV uses SHA256 hashing instead of truncation to avoid
// collisions (GCNV has a stricter 63-char limit). The hash approach is borrowed from ONTAP's
// getUniqueNodeSpecificSubsystemName (NVMe subsystems) which also uses SHA256 when names exceed
// limits. GCNV also requires additional normalization (lowercase, char filtering) that ONTAP
// igroups don't need.
func (d *SANStorageDriver) getNodeSpecificHostGroupName(nodeName string) string {
	const maxLength = 63
	const prefix = "trident-node-"

	// Try the full name first
	fullName := fmt.Sprintf("%s%s-%s", prefix, nodeName, d.tridentUUID)

	// Normalize: convert to lowercase, replace invalid chars with hyphens
	normalized := strings.ToLower(fullName)
	normalized = strings.ReplaceAll(normalized, "_", "-")

	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	var builder strings.Builder
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	normalized = builder.String()

	// Remove consecutive hyphens
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}

	// NOTE: We don't need a separate "ensure starts with a letter" check here:
	// - The non-hashed path always starts with `prefix` ("trident-node-"), which starts with 't'.
	// - The hashed path always starts with `prefix + "t"`.
	// The construction guarantees a non-empty result because the prefix and UUID are always present.
	// Note: nodeName is assumed to be non-empty by callers; an empty nodeName would produce a
	// valid but non-unique identifier. After hashing, the result always fits within maxLength.

	// If still too long, hash the node name + UUID and use that
	if len(normalized) > maxLength {
		// Create a hash of nodeName + UUID for uniqueness
		hashInput := fmt.Sprintf("%s-%s", nodeName, d.tridentUUID)
		hash := sha256.Sum256([]byte(hashInput))
		hashStr := hex.EncodeToString(hash[:])

		// Use prefix + first part of hash, ensuring it starts with a letter
		// Reserve space for prefix + 1 letter + hash.
		availableForHash := maxLength - len(prefix) - 1
		normalized = prefix + "t" + hashStr[:availableForHash]
	}

	// Ensure it doesn't end with a hyphen
	normalized = strings.TrimRight(normalized, "-")

	return normalized
}

// extractNodeNameFromHostGroupName extracts the Kubernetes node name from a host group name.
func (d *SANStorageDriver) extractNodeNameFromHostGroupName(hostGroupName string) string {
	// Extract node name from host group name format: "trident-node-{nodeName}-{uuid}"
	// or full path: "projects/.../hostGroups/trident-node-{nodeName}-{uuid}"
	const prefix = "trident-node-"

	// Extract just the name part if it's a full path
	name := hostGroupName
	if idx := strings.LastIndex(hostGroupName, "/"); idx >= 0 {
		name = hostGroupName[idx+1:]
	}

	// Check if it starts with the prefix
	if !strings.HasPrefix(name, prefix) {
		return ""
	}

	// Remove the prefix
	name = name[len(prefix):]

	// The UUID is at the end and has a known format (from tridentUUID)
	// Format: {nodeName}-{uuid}
	// The UUID format is: {8hex}-{4hex}-{4hex}-{4hex}-{12hex}
	// We can find the UUID by looking for the pattern at the end
	// But a simpler approach: find the last occurrence of the UUID pattern
	// Since we have d.tridentUUID, we can use it to find where the UUID starts
	uuidLen := len(d.tridentUUID)
	if len(name) <= uuidLen {
		return ""
	}

	// Check if the name ends with the UUID
	if !strings.HasSuffix(name, d.tridentUUID) {
		return ""
	}

	// Extract node name (everything before the UUID and the hyphen before it)
	nodeName := name[:len(name)-uuidLen-1] // -1 for the hyphen before UUID
	return nodeName
}

// getTelemetryLabels returns a map of labels to be applied to volumes for telemetry purposes.
func (d *SANStorageDriver) getTelemetryLabels(ctx context.Context) map[string]string {
	return map[string]string{
		"version":          d.fixGCPLabelValue(d.telemetry.TridentVersion),
		"backend_uuid":     d.fixGCPLabelValue(d.telemetry.TridentBackendUUID),
		"platform":         d.fixGCPLabelValue(d.telemetry.Platform),
		"platform_version": d.fixGCPLabelValue(d.telemetry.PlatformVersion),
		"plugin":           d.fixGCPLabelValue(d.telemetry.Plugin),
	}
}

// updateTelemetryLabels updates a volume's labels to include the standard telemetry labels.
func (d *SANStorageDriver) updateTelemetryLabels(ctx context.Context, volume *api.Volume) map[string]string {
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
func (d *SANStorageDriver) fixGCPLabelKey(s string) (string, bool) {
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
	s = convert.TruncateString(s, api.MaxLabelLength)

	return s, true
}

// fixGCPLabelValue accepts a label value and modifies it to satisfy GCP label value rules.
func (d *SANStorageDriver) fixGCPLabelValue(s string) string {
	// Convert the string to lowercase
	s = strings.ToLower(s)

	// Replace all disallowed characters with underscores
	s = gcpLabelRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("_", len(m))
	})

	// Shorten the string to a maximum of 63 characters
	s = convert.TruncateString(s, api.MaxLabelLength)

	return s
}

// waitForVolumeCreate waits for volume creation to complete by reaching the Ready state. If the
// volume reaches a terminal state (Error), the volume is deleted. If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
func (d *SANStorageDriver) waitForVolumeCreate(ctx context.Context, volume *api.Volume) error {
	state, err := d.API.WaitForVolumeState(
		ctx, volume, api.VolumeStateReady, []string{api.VolumeStateError}, d.volumeCreateTimeout)
	if err != nil {

		logFields := LogFields{"volume": volume.Name}

		switch state {

		case api.VolumeStateUnspecified, api.VolumeStateCreating:
			Logc(ctx).WithFields(logFields).Debugf("Volume is in %s state.", state)
			return errors.VolumeCreatingError(err.Error())

		case api.VolumeStateDeleting:
			// Wait for deletion to complete
			_, errDelete := d.API.WaitForVolumeState(
				ctx, volume, api.VolumeStateDeleted, []string{api.VolumeStateError}, d.volumeCreateTimeout)
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
			}
			return errDelete

		case api.VolumeStateError:
			// Delete a failed volume
			errDelete := d.API.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
				return errDelete
			} else {
				Logc(ctx).WithField("volume", volume.Name).Info("Volume deleted.")
			}

		case "":
			// State will be empty if the volume could not be found or there was an API error
			Logc(ctx).WithFields(logFields).WithError(err).Error("Volume state is unknown.")
			return err

		default:
			Logc(ctx).WithFields(logFields).Errorf("unexpected volume state %s found for volume", state)
		}
	}

	return nil
}

// CreatePrepare is called prior to volume creation. Its role is to set the internal volume name.
func (d *SANStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig, pool)
}

// Create creates a new volume.
func (d *SANStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "Create",
		"Type":   "SANStorageDriver",
		"name":   name,
		"size":   volConfig.Size,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Discover GCNV resources (capacity pools, etc.)
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
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

	// Check if volume already exists
	volumeExists, existingVolume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		// Volume exists - check its state for proper idempotent handling
		if existingVolume.State == api.VolumeStateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.VolumeStateCreating, api.VolumeStateReady))
		}

		Logc(ctx).WithFields(LogFields{
			"name":  name,
			"state": existingVolume.State,
		}).Debug("Volume already exists.")

		return drivers.NewVolumeExistsError(name)
	}

	// Parse size
	sizeBytes, err := strconv.ParseUint(volConfig.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid volume size: %v", err)
	}

	// Ensure the volume size meets minimum requirements
	if err = drivers.CheckMinVolumeSize(sizeBytes, MinimumVolumeSizeBytes); err != nil {
		return err
	}

	// NOTE: Snapshot reserve is applied later after parsing; size checks below use the base size.
	// The actualSizeBytes calculation will be done after snapshot reserve is parsed.

	Logc(ctx).WithFields(LogFields{
		"requestedBytes": sizeBytes,
		"requestedGiB":   float64(sizeBytes) / float64(api.GiBBytes),
	}).Debug("Parsed volume size request.")

	// Check for a supported file system type (volConfig takes precedence, e.g. from PVC annotation)
	fstype := volConfig.FileSystem
	if fstype == "" {
		fstype = pool.InternalAttributes()[FileSystemType]
	}
	fstype, err = drivers.CheckSupportedFilesystem(ctx, fstype, name)
	if err != nil {
		return err
	}
	volConfig.FileSystem = fstype

	// Get service level from storage pool
	serviceLevel := pool.InternalAttributes()[ServiceLevel]
	volConfig.ServiceLevel = serviceLevel

	// Get snapshot reserve (volConfig takes precedence, e.g. from PVC annotation)
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
	volConfig.SnapshotReserve = snapshotReserve

	// Get the volume size based on the snapshot reserve
	sizeWithReserveBytes := drivers.CalculateVolumeSizeBytes(ctx, name, sizeBytes, snapshotReserveInt)

	// Check configured maximum volume size (if any)
	_, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeWithReserveBytes, d.Config.CommonStorageDriverConfig)
	if err != nil {
		return err
	}

	// Calculate actual size to request from GCNV (rounds up to whole GiB + 1 GiB buffer)
	actualSizeBytes, err := calculateActualSizeBytes(int64(sizeWithReserveBytes))
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"requestedBytes":       sizeBytes,
		"snapshotReserve":      snapshotReserve,
		"sizeWithReserveBytes": sizeWithReserveBytes,
		"actualSizeBytes":      actualSizeBytes,
		"actualSizeGiB":        actualSizeBytes / api.GiBBytes,
	}).Debug("Adjusted volume size to include snapshot reserve and ensure usable space.")

	// Find matching capacity pools
	cPools := d.API.CapacityPoolsForStoragePool(ctx, pool, serviceLevel, drivers.TieringPolicyNone)

	// Filter capacity pools based on requisite topology
	cPools = d.API.FilterCapacityPoolsOnTopology(ctx, cPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies)

	if len(cPools) == 0 {
		return fmt.Errorf("no GCNV storage pools found for Trident pool %s", pool.Name())
	}

	// Prepare labels (used for all capacity pool attempts)
	labels := d.getTelemetryLabels(ctx)

	// Add pool labels (user-defined labels from backend config)
	for k, v := range pool.GetLabels(ctx, "") {
		if key, keyOK := d.fixGCPLabelKey(k); keyOK {
			labels[key] = d.fixGCPLabelValue(v)
		}
		if len(labels) > api.MaxLabelCount {
			break
		}
	}

	// Add volume-specific labels (fstype and formatOptions)
	if key, ok := d.fixGCPLabelKey(TridentLabelFSType); ok {
		labels[key] = d.fixGCPLabelValue(volConfig.FileSystem)
	}
	if volConfig.FormatOptions != "" {
		if key, ok := d.fixGCPLabelKey(TridentLabelFormatOptions); ok {
			labels[key] = d.fixGCPLabelValue(volConfig.FormatOptions)
		}
	}

	// Try each capacity pool until one succeeds
	var createErr error
	for _, cPool := range cPools {

		Logc(ctx).WithFields(LogFields{
			"capacityPool":    cPool.Name,
			"name":            name,
			"requestedSize":   sizeBytes,
			"snapshotReserve": snapshotReserve,
			"actualSizeBytes": actualSizeBytes,
		}).Debug("Attempting to create LUN in capacity pool.")

		// Create block volume (LUN) with empty host groups
		createReq := &api.VolumeCreateRequest{
			Name:            name,
			CreationToken:   name,
			CapacityPool:    cPool.Name,
			SizeBytes:       actualSizeBytes,
			Labels:          labels,
			SnapshotReserve: snapshotReservePtr,
			ProtocolTypes:   []string{api.ProtocolTypeISCSI},
			OSType:          api.OSTypeLinux,
		}

		volume, err := d.API.CreateVolume(ctx, createReq)
		if err != nil {
			errMessage := fmt.Sprintf("GCNV pool %s; error creating LUN %s: %v", cPool.Name, name, err)
			Logc(ctx).Error(errMessage)
			createErr = multierr.Append(createErr, errors.New(errMessage))
			continue
		}

		// Always save the full GCP ID (projects/.../locations/.../volumes/...)
		volConfig.InternalID = volume.FullName

		// Report the actual allocated size so PV reflects what GCNV created (and bills for)
		volConfig.Size = strconv.FormatUint(uint64(actualSizeBytes), 10)

		Logc(ctx).WithField("capacityPool", cPool.Name).Debug("LUN create request issued.")

		// Wait for creation to complete so that the volume is ready for publishing
		return d.waitForVolumeCreate(ctx, volume)
	}

	// All capacity pools failed
	return fmt.Errorf("could not create LUN in any capacity pool: %w", createErr)
}

// Destroy deletes a volume.
func (d *SANStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) (err error) {
	name := volConfig.InternalName
	fields := LogFields{"Method": "Destroy", "Type": "SANStorageDriver", "name": name}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Discover GCNV resources
	if err = d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
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
	} else if extantVolume.State == api.VolumeStateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.API.WaitForVolumeState(ctx, extantVolume, api.VolumeStateDeleted,
			[]string{api.VolumeStateError}, d.volumeCreateTimeout)
		return err
	}

	// Delete the volume
	if err = d.API.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	Logc(ctx).WithField("volume", extantVolume.Name).Info("Volume deleted.")

	// Wait for deletion to complete
	_, err = d.API.WaitForVolumeState(ctx, extantVolume, api.VolumeStateDeleted,
		[]string{api.VolumeStateError}, d.defaultTimeout())
	return err
}

// deleteAutomaticSnapshot deletes a snapshot that was created automatically during volume clone creation.
// An automatic snapshot is detected by the presence of CloneSourceSnapshotInternal in the volume config
// while CloneSourceSnapshot is not set. This method is called after the volume has been deleted, and it
// will only attempt snapshot deletion if the clone volume deletion completed without error. This is a
// best-effort method, and any errors encountered will be logged but not returned.
func (d *SANStorageDriver) deleteAutomaticSnapshot(
	ctx context.Context, volDeleteError error, volConfig *storage.VolumeConfig,
) {
	snapshot := volConfig.CloneSourceSnapshotInternal
	cloneSourceName := volConfig.CloneSourceVolumeInternal
	cloneName := volConfig.InternalName
	fields := LogFields{
		"Method":          "deleteAutomaticSnapshot",
		"Type":            "SANStorageDriver",
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
	if err = d.API.DeleteSnapshot(ctx, sourceVolume, sourceSnapshot, api.DefaultTimeout); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Errorf("Automatic snapshot could not be " +
			"cleaned up and must be manually deleted.")
	}
}

// Rename changes the name of a volume.
// Rename changes the name of a volume. Not supported by this driver.
func (d *SANStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "SANStorageDriver",
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

// CreateClone clones an existing volume. If a snapshot is not specified, one is created.
func (d *SANStorageDriver) CreateClone(
	ctx context.Context, sourceVolConfig, cloneConfig *storage.VolumeConfig, _ storage.Pool,
) error {
	name := cloneConfig.InternalName
	fields := LogFields{
		"Method":         "CreateClone",
		"Type":           "SANStorageDriver",
		"name":           name,
		"sourceVolume":   cloneConfig.CloneSourceVolumeInternal,
		"sourceSnapshot": cloneConfig.CloneSourceSnapshot,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	// Discover GCNV resources (capacity pools, volumes, etc.)
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Make sure we got a valid name
	if err := d.validateVolumeName(cloneConfig.Name); err != nil {
		return err
	}

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(name); err != nil {
		return err
	}

	// Check if clone volume already exists (for idempotency)
	cloneExists, existingVolume, err := d.API.VolumeExists(ctx, cloneConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if cloneExists {
		// Clone volume exists - check its state for proper idempotent handling
		if existingVolume.State == api.VolumeStateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.VolumeStateCreating, api.VolumeStateReady))
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get source volume (uses InternalID if available for faster lookup)
	sourceVolume, err := d.API.Volume(ctx, sourceVolConfig)
	if err != nil {
		return fmt.Errorf("could not find source volume: %v", err)
	}

	var snapshot *api.Snapshot

	if cloneConfig.CloneSourceSnapshotInternal != "" {
		// Get the source snapshot
		snapshot, err = d.API.SnapshotForVolume(ctx, sourceVolume, cloneConfig.CloneSourceSnapshotInternal)
		if err != nil {
			return fmt.Errorf("could not find snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if snapshot.State != api.SnapshotStateReady {
			return fmt.Errorf("source snapshot state is %s, it must be %s",
				snapshot.State, api.SnapshotStateReady)
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot": cloneConfig.CloneSourceSnapshotInternal,
			"source":   sourceVolume.Name,
		}).Debug("Found source snapshot.")

	} else {
		// No source snapshot specified, so create an intermediate one for direct PVC clone
		snapName := "snap-" + strings.ToLower(time.Now().UTC().Format(storage.SnapshotNameFormat))

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapName,
			"source":   sourceVolume.Name,
		}).Debug("Creating intermediate snapshot for direct PVC clone.")

		snapshot, err = d.API.CreateSnapshot(ctx, sourceVolume, snapName, api.SnapshotTimeout)
		if err != nil {
			return fmt.Errorf("could not create intermediate snapshot: %v", err)
		}

		// Re-fetch the snapshot to populate the properties after create has completed
		snapshot, err = d.API.SnapshotForVolume(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not retrieve newly-created snapshot: %v", err)
		}

		// Save the snapshot name in the volume config so we can auto-delete it later
		cloneConfig.CloneSourceSnapshotInternal = snapshot.Name

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapshot.Name,
			"source":   sourceVolume.Name,
		}).Debug("Created intermediate snapshot for direct PVC clone.")

		// Defer cleanup of intermediate snapshot - runs on success or failure
		defer func() {
			if delErr := d.API.DeleteSnapshot(ctx, sourceVolume, snapshot, api.DefaultTimeout); delErr != nil {
				Logc(ctx).WithFields(LogFields{
					"snapshot": snapshot.Name,
					"source":   sourceVolume.Name,
				}).WithError(delErr).Debug("Could not delete intermediate snapshot.")
			}
		}()
	}

	// Parse size
	sizeBytes, err := strconv.ParseUint(cloneConfig.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid volume size: %v", err)
	}

	// Create labels
	labels := d.getTelemetryLabels(ctx)
	// Add volume-specific labels (fstype and formatOptions) - sanitize keys for GCP API
	if key, ok := d.fixGCPLabelKey(TridentLabelFSType); ok {
		labels[key] = d.fixGCPLabelValue(cloneConfig.FileSystem)
	}
	if cloneConfig.FormatOptions != "" {
		if key, ok := d.fixGCPLabelKey(TridentLabelFormatOptions); ok {
			labels[key] = d.fixGCPLabelValue(cloneConfig.FormatOptions)
		}
	}

	// Clone size must be at least as large as the source volume.
	// Use source volume's actual size (which includes the +1 GiB buffer from Create).
	cloneSizeBytes := sourceVolume.SizeBytes
	if int64(sizeBytes) > cloneSizeBytes {
		// If user requests larger than source, apply the buffer to the requested size
		var err error
		cloneSizeBytes, err = calculateActualSizeBytes(int64(sizeBytes))
		if err != nil {
			return err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"creationToken":   name,
		"sourceVolume":    sourceVolume.CreationToken,
		"sourceSnapshot":  snapshot.Name,
		"snapshotReserve": sourceVolume.SnapshotReserve,
	}).Debug("Cloning volume.")

	// Create clone from snapshot with empty host groups
	createReq := &api.VolumeCreateRequest{
		Name:            name,
		CreationToken:   name,
		CapacityPool:    sourceVolume.CapacityPool,
		SizeBytes:       cloneSizeBytes,
		Labels:          labels,
		SnapshotReserve: convert.ToPtr(sourceVolume.SnapshotReserve),
		SnapshotID:      snapshot.FullName, // Use full resource path for v1 protobuf SDK
		ProtocolTypes:   []string{api.ProtocolTypeISCSI},
		OSType:          api.OSTypeLinux,
	}

	clone, err := d.API.CreateVolume(ctx, createReq)
	if err != nil {
		return fmt.Errorf("could not create clone: %v", err)
	}

	// Always save the full GCP ID for efficient future lookups
	cloneConfig.InternalID = clone.FullName

	// Report the actual allocated size so PV reflects what GCNV created
	cloneConfig.Size = strconv.FormatUint(uint64(cloneSizeBytes), 10)

	// Wait for clone creation to complete so that the volume is ready for publishing
	return d.waitForVolumeCreate(ctx, clone)
}

// Import finds an existing volume and makes it available for containers.
func (d *SANStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "SANStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get existing volume
	volume, err := d.API.VolumeByName(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not find volume to import: %v", err)
	}

	// Ensure volume is in a valid state
	if volume.State != api.VolumeStateReady {
		return fmt.Errorf("volume %s is in state %s and is not available", originalName, volume.State)
	}

	// Verify it's a block/iSCSI volume.
	//
	// In GCNV, iSCSI (block) volumes surface with either iSCSI in `ProtocolTypes` or populated `BlockDevices`.
	// Although we generally expect a single LUN/block device per volume today, the wire format is modeled as a list,
	// so we treat a non-empty list as "this is a block volume". As a fallback (and to mirror API conventions),
	// also accept volumes whose ProtocolTypes include ISCSI.
	isISCSI := len(volume.BlockDevices) > 0
	if !isISCSI && len(volume.ProtocolTypes) > 0 {
		for _, proto := range volume.ProtocolTypes {
			if proto == api.ProtocolTypeISCSI {
				isISCSI = true
				break
			}
		}
	}
	if !isISCSI {
		return fmt.Errorf("volume %s is not an iSCSI volume", originalName)
	}

	// Ensure the volume may be imported by a capacity pool managed by this backend
	if err = d.API.EnsureVolumeInValidCapacityPool(ctx, volume); err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(uint64(volume.SizeBytes), 10)

	// The GCNV creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = originalName

	// Always save the full GCP ID
	volConfig.InternalID = volume.FullName

	Logc(ctx).WithFields(LogFields{
		"creationToken": volume.CreationToken,
		"managed":       !volConfig.ImportNotManaged,
		"state":         volume.State,
		"capacityPool":  volume.CapacityPool,
		"sizeBytes":     volume.SizeBytes,
	}).Debug("Found volume to import.")

	// Modify the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// Update the volume labels with telemetry information
		labels := d.updateTelemetryLabels(ctx, volume)

		// Update volume labels using UpdateVolume API (preserves block_devices)
		updateReq := &api.VolumeUpdateRequest{
			Labels:    labels,
			FieldMask: []string{"labels"},
		}
		if _, err := d.API.UpdateSANVolume(ctx, volume, updateReq); err != nil {
			Logc(ctx).WithField("originalName", originalName).WithError(err).Error(
				"Could not import volume, volume label update failed.")
			return fmt.Errorf("could not import volume %s, volume label update failed; %v", originalName, err)
		}

		Logc(ctx).WithFields(LogFields{
			"name":          volume.Name,
			"creationToken": volume.CreationToken,
			"labels":        labels,
		}).Info("Volume labels updated during import.")

		// Wait for the update to complete
		if _, err = d.API.WaitForVolumeState(
			ctx, volume, api.VolumeStateReady, []string{api.VolumeStateError}, d.defaultTimeout()); err != nil {
			return fmt.Errorf("could not import volume %s; %v", originalName, err)
		}
	}

	return nil
}

// Get tests for the existence of a volume.
func (d *SANStorageDriver) Get(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName
	fields := LogFields{"Method": "Get", "Type": "SANStorageDriver", "name": name}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	_, err := d.API.VolumeByName(ctx, name)
	return err
}

// Resize increases a volume's quota.
func (d *SANStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":    "Resize",
		"Type":      "SANStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get volume (uses InternalID if available for faster lookup)
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return err
	}

	// If the volume state isn't Ready, return an error
	if volume.State != api.VolumeStateReady {
		return fmt.Errorf("volume %s state is %s, not %s", name, volume.State, api.VolumeStateReady)
	}

	// Include the snapshot reserve in the new size
	if volume.SnapshotReserve > math.MaxInt {
		return fmt.Errorf("snapshot reserve too large")
	}
	sizeWithReserveBytes := drivers.CalculateVolumeSizeBytes(ctx, name, sizeBytes, int(volume.SnapshotReserve))

	// Calculate actual size to request from GCNV (rounds up to whole GiB + 1 GiB buffer)
	actualSizeBytes, err := calculateActualSizeBytes(int64(sizeWithReserveBytes))
	if err != nil {
		return err
	}

	// If the volume is already the requested size, there's nothing to do
	if actualSizeBytes == volume.SizeBytes {
		volConfigSize := strconv.FormatUint(sizeBytes, 10)
		if volConfigSize != volConfig.Size {
			volConfig.Size = volConfigSize
		}
		return nil
	}

	// Make sure we're not shrinking the volume
	if actualSizeBytes < volume.SizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d",
			actualSizeBytes, volume.SizeBytes)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	_, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"requestedBytes":  sizeBytes,
		"requestedGiB":    float64(sizeBytes) / float64(api.GiBBytes),
		"actualSizeBytes": actualSizeBytes,
		"actualSizeGiB":   actualSizeBytes / api.GiBBytes,
	}).Debug("Adjusted resize to ensure usable space meets requested capacity.")

	if err = d.API.ResizeVolume(ctx, volume, actualSizeBytes); err != nil {
		return err
	}

	// Report the actual allocated size to match what GCNV shows (and bills for)
	volConfig.Size = strconv.FormatUint(uint64(actualSizeBytes), 10)
	return nil
}

// Publish the volume to the host specified in publishInfo. This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "Publish",
		"Type":   "SANStorageDriver",
		"name":   name,
		"node":   publishInfo.HostName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Discover GCNV resources (volumes, host groups, etc.)
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Acquire mutexes in order to prevent deadlock
	d.lunMutex.Lock(name)
	defer d.lunMutex.Unlock(name)

	d.hostGroupMutex.Lock(publishInfo.HostName)
	defer d.hostGroupMutex.Unlock(publishInfo.HostName)

	// Get volume (uses InternalID if available for faster lookup)
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume: %v", err)
	}
	// Determine node IQN
	// In CSI architecture, Publish runs in the controller context and the orchestrator
	// must provide the node's IQN via publishInfo.HostIQN.
	// Note: Docker mode (publishInfo.Localhost == true) is not supported (see design doc).
	if publishInfo.Localhost {
		return fmt.Errorf("Docker mode (Localhost=true) is not supported by GCNV SAN driver; only CSI mode is supported")
	}

	var nodeIQN string
	if len(publishInfo.HostIQN) == 0 || publishInfo.HostIQN[0] == "" {
		// Generate a synthetic IQN if none is provided (needed for defensive checks)
		// Format: iqn.1994-05.com.netapp:trident-node-<node-name-hash>
		hash := sha256.Sum256([]byte(publishInfo.HostName))
		hashStr := hex.EncodeToString(hash[:16]) // 16 bytes (128 bits): unique enough and keeps IDs under 63 chars
		nodeIQN = fmt.Sprintf("iqn.1994-05.com.netapp:trident-node-%s", hashStr)
		Logc(ctx).WithFields(LogFields{
			"node":      publishInfo.HostName,
			"generated": nodeIQN,
		}).Debug("No host IQN provided for node, generating synthetic IQN for GCNV.")
	} else {
		nodeIQN = publishInfo.HostIQN[0]
	}

	// Validate IQN format: must be in format "iqn.yyyy-mm.domain:string"
	if !strings.HasPrefix(nodeIQN, "iqn.") {
		Logc(ctx).WithFields(LogFields{
			"node": publishInfo.HostName,
			"iqn":  nodeIQN,
		}).Error("Node IQN does not start with 'iqn.', GCNV requires IQN format.")
		return fmt.Errorf("invalid IQN format for node %s: must start with 'iqn.' but got '%s'", publishInfo.HostName, nodeIQN)
	}

	Logc(ctx).WithFields(LogFields{
		"node": publishInfo.HostName,
		"iqn":  nodeIQN,
	}).Debug("Using node IQN for host group.")

	// Get or create node-specific host group
	hostGroupName := d.getNodeSpecificHostGroupName(publishInfo.HostName)
	hostGroup, err := d.API.HostGroupByName(ctx, hostGroupName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// Host group doesn't exist - create it
			createReq := &api.HostGroupCreateRequest{
				ResourceID:  hostGroupName,
				Hosts:       []string{nodeIQN},
				OSType:      api.OSTypeLinux,
				Type:        api.HostGroupTypeISCSIInitiator,
				Description: fmt.Sprintf("Trident host group for node %s", publishInfo.HostName),
			}
			hostGroup, err = d.API.CreateHostGroup(ctx, createReq)
			if err != nil {
				return fmt.Errorf("could not create host group: %v", err)
			}
		} else {
			// Other error (network, auth, API failure, etc.)
			return fmt.Errorf("could not get host group: %v", err)
		}
	} else {
		// Host group exists - ensure IQN is present
		found := false
		for _, iqn := range hostGroup.Hosts {
			if iqn == nodeIQN {
				found = true
				break
			}
		}
		if !found {
			if err := d.API.AddInitiatorsToHostGroup(ctx, hostGroup, []string{nodeIQN}); err != nil {
				return fmt.Errorf("could not add IQN to host group: %v", err)
			}
		}
	}

	// Check if volume is already mapped to this host group
	mappedGroups, err := d.API.VolumeMappedHostGroups(ctx, volume.FullName)
	if err != nil {
		return fmt.Errorf("could not get mapped host groups: %v", err)
	}

	alreadyMapped := false
	for _, gid := range mappedGroups {
		if gid == hostGroup.Name {
			alreadyMapped = true
			break
		}
	}

	if !alreadyMapped {
		if err := d.API.AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name); err != nil {
			return fmt.Errorf("could not map volume to host group: %v", err)
		}
	}

	// Get iSCSI target info
	targetInfo, err := d.API.ISCSITargetInfo(ctx, volume)
	if err != nil {
		return fmt.Errorf("could not get iSCSI target info: %v", err)
	}

	// TODO(GCNV API): When GCNV API exposes the mapped LUN number per host group, use it here
	// instead of hardcoding 0. The LUN number should come from the volume's block device mapping.
	// GCNV auto-assigns LUN IDs and doesn't expose them via API.
	// Set 0 as placeholder; node-side code will detect serial mismatch and discover the actual LUN.
	publishInfo.IscsiLunNumber = 0

	// GCNV returns the serial number as a hex-encoded string, but the device presents it as ASCII.
	// We need to decode the hex to ASCII so it matches what we read from VPD page 0x80.
	// If this fails, node-side attach will fail to find the device by serial, so fail fast here.
	serialBytes, err := hex.DecodeString(volume.SerialNumber)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"serial": volume.SerialNumber,
			"error":  err,
		}).Error("GCNV serial is not hex-encoded ASCII; cannot decode to match VPD page 0x80.")
		return fmt.Errorf("could not decode GCNV serial number %q: %v", volume.SerialNumber, err)
	}
	publishInfo.IscsiLunSerial = string(serialBytes)
	Logc(ctx).WithFields(LogFields{
		"hexSerial":   volume.SerialNumber,
		"asciiSerial": string(serialBytes),
	}).Debug("Decoded GCNV serial number from hex to ASCII")

	publishInfo.IscsiTargetPortal = targetInfo.TargetPortal
	publishInfo.IscsiPortals = targetInfo.Portals
	publishInfo.SANType = "iscsi"
	publishInfo.UseCHAP = false
	publishInfo.SharedTarget = false
	publishInfo.IscsiIgroup = hostGroupName

	// TODO(GCNV API): When GCNV API exposes the target IQN directly, use it here instead of
	// the fallback IQN. Remove the discovery workaround and the trace log below.
	// GCNV API does not provide the target IQN. Set a fallback IQN (derived from serial number).
	// The actual target IQN will be discovered via iSCSI SendTargets during volume attach on the node.
	publishInfo.IscsiTargetIQN = targetInfo.TargetIQN

	Logc(ctx).WithFields(LogFields{
		"fallbackIQN": publishInfo.IscsiTargetIQN,
		"portals":     publishInfo.IscsiPortals,
		"serial":      volume.SerialNumber,
		"volumeName":  volume.Name,
	}).Trace("Using fallback IQN; actual IQN will be discovered via iSCSI SendTargets during volume attach.")

	// Set filesystem type from labels (use transformed key since GCP labels don't allow dots/slashes)
	if fstypeKey, ok := d.fixGCPLabelKey(TridentLabelFSType); ok {
		if fstype, ok := volume.Labels[fstypeKey]; ok {
			publishInfo.FilesystemType = fstype
		}
	}

	// Set format options from labels (use transformed key)
	if formatOptionsKey, ok := d.fixGCPLabelKey(TridentLabelFormatOptions); ok {
		if formatOptions, ok := volume.Labels[formatOptionsKey]; ok {
			publishInfo.FormatOptions = formatOptions
		}
	}

	// Add XFS nouuid mount option if needed
	if publishInfo.FilesystemType == "xfs" {
		if publishInfo.MountOptions == "" {
			publishInfo.MountOptions = "nouuid"
		} else {
			publishInfo.MountOptions = publishInfo.MountOptions + ",nouuid"
		}
	}

	return nil
}

// Unpublish removes volume access from the host specified in publishInfo.
func (d *SANStorageDriver) Unpublish(ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "Unpublish",
		"Type":   "SANStorageDriver",
		"name":   name,
		"node":   publishInfo.HostName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	// Discover GCNV resources (volumes, host groups, etc.)
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Acquire mutexes
	d.lunMutex.Lock(name)
	defer d.lunMutex.Unlock(name)

	d.hostGroupMutex.Lock(publishInfo.HostName)
	defer d.hostGroupMutex.Unlock(publishInfo.HostName)

	// Get volume (uses InternalID if available for faster lookup)
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithField("volume", name).Debug("Volume not found, assuming already unpublished.")
			return nil
		}
		return fmt.Errorf("could not find volume: %v", err)
	} // Get host group
	hostGroupName := d.getNodeSpecificHostGroupName(publishInfo.HostName)
	hostGroup, err := d.API.HostGroupByName(ctx, hostGroupName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithField("hostGroup", hostGroupName).Debug("Host group not found, assuming already unpublished.")
			return nil
		}
		return fmt.Errorf("could not find host group: %v", err)
	}

	// Unmap volume from host group
	if err := d.API.RemoveHostGroupFromVolume(ctx, volume.FullName, hostGroup.Name); err != nil {
		return fmt.Errorf("could not unmap volume from host group: %v", err)
	}

	// Check if host group has any remaining volumes
	volumes, err := d.API.HostGroupVolumes(ctx, hostGroup.Name)
	if err != nil {
		return fmt.Errorf("could not check host group volumes: %v", err)
	}

	// Delete host group if no volumes remain
	if len(volumes) == 0 {
		if err := d.API.DeleteHostGroup(ctx, hostGroup); err != nil {
			return fmt.Errorf("could not delete empty host group: %v", err)
		}
	}

	return nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *SANStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Check if volume exists (uses InternalID if available for faster lookup)
	volumeExists, volume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Check if snapshot already exists (for idempotency)
	snapshot, err := d.API.SnapshotForVolume(ctx, volume, snapConfig.InternalName)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, fmt.Errorf("error checking for existing snapshot %s; %w", snapConfig.InternalName, err)
	}

	// Create the snapshot if it doesn't exist
	if snapshot == nil {
		snapshot, err = d.API.CreateSnapshot(ctx, volume, snapConfig.InternalName, api.SnapshotTimeout)
		if err != nil {
			return nil, fmt.Errorf("could not create snapshot: %v", err)
		}
	}

	// At this point, snapshot should not be nil
	if snapshot.State == api.SnapshotStateReady {
		Logc(ctx).WithFields(fields).Info("Snapshot created or already exists and is Ready.")

		return &storage.Snapshot{
			Config:    snapConfig,
			Created:   snapshot.Created.Format(time.RFC3339),
			SizeBytes: volume.SizeBytes,
			State:     storage.SnapshotStateOnline,
		}, nil
	}

	fields["state"] = snapshot.State
	Logc(ctx).WithFields(fields).Debug("Snapshot already exists but is not Ready.")

	return nil, fmt.Errorf("snapshot %s already exists but is %s, not %s",
		snapConfig.InternalName, snapshot.State, api.SnapshotStateReady)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get the volume (uses InternalID if available for faster lookup)
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

	// Wait for volume to return to Ready state
	_, err = d.API.WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError, api.VolumeStateDeleting, api.VolumeStateDeleted}, api.DefaultSDKTimeout,
	)
	return err
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *SANStorageDriver) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get volume (uses InternalID if available for faster lookup)
	volumeExists, volume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		// If the volume doesn't exist, neither does the snapshot
		Logc(ctx).WithFields(LogFields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}).Debug("Volume for snapshot not found.")
		return nil
	}

	// Get snapshot
	snapshot, err := d.API.SnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithField("snapshot", internalSnapName).Debug("Snapshot already deleted.")
			return nil
		}
		return err
	}

	// Delete snapshot
	return d.API.DeleteSnapshot(ctx, volume, snapshot, api.DefaultTimeout)
}

// ReconcileNodeAccess reconciles host group state with cluster nodes.
// The orchestrator filters TridentVolumePublication CRs to determine which nodes have volumes
// on this backend, and passes the filtered node list to this method.
// This method ensures per-node host groups exist for active nodes and cleans up orphaned groups.
func (d *SANStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*models.Node, _, _ string) error {
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	// Discover GCNV resources (host groups, etc.)
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get all host groups
	hostGroups, err := d.API.HostGroups(ctx)
	if err != nil {
		return fmt.Errorf("could not list host groups: %v", err)
	}

	// Build map of current nodes
	nodeMap := make(map[string]*models.Node)
	for _, node := range nodes {
		nodeMap[node.Name] = node
	}

	// Process each host group
	for _, hg := range hostGroups {
		nodeName := d.extractNodeNameFromHostGroupName(hg.Name)
		if nodeName == "" {
			// Not a Trident-managed host group
			continue
		}

		node, exists := nodeMap[nodeName]
		if exists {
			// Node exists - ensure IQNs are up to date
			if node.IQN != "" {
				// Check if IQN needs updating
				needsUpdate := len(hg.Hosts) != 1 || hg.Hosts[0] != node.IQN

				if needsUpdate {
					// Acquire lock for this host group to prevent races with Publish/Unpublish
					d.hostGroupMutex.Lock(nodeName)
					// Update host group IQNs
					if err := d.API.UpdateHostGroup(ctx, hg, []string{node.IQN}); err != nil {
						Logc(ctx).WithError(err).WithField("hostGroup", hg.Name).Debug("Could not update host group IQNs.")
					}
					d.hostGroupMutex.Unlock(nodeName)
				}
			}
		} else {
			// Node no longer exists - check if we should delete the host group.
			// First do a quick unlocked check to avoid taking locks for host groups that clearly have volumes.
			volumes, err := d.API.HostGroupVolumes(ctx, hg.Name)
			if err != nil {
				Logc(ctx).WithError(err).WithField("hostGroup", hg.Name).Debug("Could not check host group volumes.")
				continue
			}

			if len(volumes) == 0 {
				// Acquire lock for this host group to prevent races with Publish/Unpublish
				d.hostGroupMutex.Lock(nodeName)

				// CRITICAL: Recheck volume count while holding lock to prevent TOCTOU race.
				// Race scenario without this check:
				// 1. Thread A (reconcile): Checks host group → 0 volumes → proceeds to delete
				// 2. Thread B (Publish): Maps volume to this host group
				// 3. Thread A: Deletes host group WITH mapped volume!
				// By rechecking here while holding the lock, we ensure no volume was mapped
				// between the initial check and deletion.
				volumes, err = d.API.HostGroupVolumes(ctx, hg.Name)
				if err != nil {
					Logc(ctx).WithError(err).WithField("hostGroup", hg.Name).Debug("Could not recheck host group volumes.")
					d.hostGroupMutex.Unlock(nodeName)
					continue
				}
				if len(volumes) > 0 {
					Logc(ctx).WithField("hostGroup", hg.Name).Debug("Host group now has volumes, skipping deletion.")
					d.hostGroupMutex.Unlock(nodeName)
					continue
				}

				// No volumes mapped - safe to delete
				if err := d.API.DeleteHostGroup(ctx, hg); err != nil {
					Logc(ctx).WithError(err).WithField("hostGroup", hg.Name).Debug("Could not delete orphaned host group.")
				}
				d.hostGroupMutex.Unlock(nodeName)
			}
		}
	}

	return nil
}

// ReconcileVolumeNodeAccess performs per-volume reconciliation of node access.
// For GCNV SAN driver with per-node host groups, all reconciliation logic is handled
// in ReconcileNodeAccess at the backend level, so this is a no-op.
func (d *SANStorageDriver) ReconcileVolumeNodeAccess(
	ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node,
) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// Remaining interface methods

// GetInternalVolumeName accepts the name of a volume being created and returns what the internal name
// should be, depending on backend requirements and Trident's operating context.
func (d *SANStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + volConfig.Name
	} else if csiRegex.MatchString(volConfig.Name) {
		// If the name is from CSI (i.e. contains a UUID), replace hyphens with underscores
		// GCP block volume names only allow lowercase letters, numbers, and underscores
		internalName := strings.ReplaceAll(volConfig.Name, "-", "_")
		Logc(ctx).WithField("volumeInternal", internalName).Debug("Using volume name as internal name.")
		return internalName
	} else {
		// Cloud volumes have strict limits on volume names, so for cloud
		// infrastructure like Trident, the simplest approach is to generate a
		// UUID-based name with a prefix that won't exceed the length limit.
		// Replace hyphens with underscores for GCP compliance.
		return "gcnv_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	}
}

// GetStorageBackendSpecs retrieves storage capabilities and registers pools with specified backend.
func (d *SANStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	backend.SetName(d.BackendName())

	for _, pool := range d.pools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	return nil
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools.
func (d *SANStorageDriver) GetStorageBackendPhysicalPoolNames(ctx context.Context) []string {
	physicalPoolNames := make([]string, 0, len(d.pools))
	for poolName := range d.pools {
		physicalPoolNames = append(physicalPoolNames, poolName)
	}
	return physicalPoolNames
}

// GetProtocol returns the protocol supported by this driver (Block).
func (d *SANStorageDriver) GetProtocol(ctx context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

// StoreConfig adds this backend's config to the persistent config struct, as needed by Trident's persistence layer.
func (d *SANStorageDriver) StoreConfig(ctx context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.GCNVConfig = &d.Config
}

// GetExternalConfig returns a clone of this backend's config, sanitized for external consumption.
func (d *SANStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.GCNVStorageDriverConfig
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

// getVolumeExternal converts a volume from the API to a VolumeExternal representation.
func (d *SANStorageDriver) getVolumeExternal(volume *api.Volume) *storage.VolumeExternal {
	internalName := volume.CreationToken
	name := internalName
	prefix := *d.Config.StoragePrefix
	if strings.HasPrefix(internalName, prefix) {
		name = internalName[len(prefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:        tridentconfig.OrchestratorAPIVersion,
		Name:           name,
		InternalName:   internalName,
		InternalID:     volume.FullName,
		Size:           strconv.FormatInt(volume.SizeBytes, 10),
		Protocol:       tridentconfig.Block,
		SnapshotPolicy: "",
		AccessMode:     tridentconfig.ReadWriteOnce,
		AccessInfo: models.VolumeAccessInfo{
			IscsiAccessInfo: models.IscsiAccessInfo{
				IscsiTargetPortal: volume.ISCSITargetPortal,
				IscsiPortals:      volume.ISCSIPortals,
				IscsiTargetIQN:    volume.ISCSITargetIQN,
				IscsiLunNumber:    int32(volume.LunID),
				IscsiLunSerial:    volume.SerialNumber,
			},
		},
		ServiceLevel: volume.ServiceLevel,
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volume.CapacityPool,
	}
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver. It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel when finished.
func (d *SANStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	fields := LogFields{"Method": "GetVolumeExternalWrappers", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetVolumeExternalWrappers")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetVolumeExternalWrappers")

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
		// Filter out non-iSCSI volumes (this driver only manages iSCSI volumes)
		isISCSI := false
		for _, proto := range volume.ProtocolTypes {
			if proto == api.ProtocolTypeISCSI {
				isISCSI = true
				break
			}
		}
		if !isISCSI {
			continue
		}

		// Filter out volumes in an unavailable state
		switch volume.State {
		case api.VolumeStateDeleting, api.VolumeStateError, api.VolumeStateDisabled:
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.CreationToken, prefix) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(volume), Error: nil}
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver.
func (d *SANStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANStorageDriver)
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

// String implements the Stringer interface for the SANStorageDriver.
// It returns a redacted version of the driver configuration to prevent
// sensitive credentials from being logged.
func (d *SANStorageDriver) String() string {
	return convert.ToStringRedacted(d, []string{"API"}, d.GetExternalConfig(context.Background()))
}

// GoString implements the GoStringer interface for the SANStorageDriver.
func (d *SANStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig.
func (d *SANStorageDriver) GetCommonConfig(ctx context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *SANStorageDriver) CanSnapshot(ctx context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	fields := LogFields{
		"Method": "CanSnapshot",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CanSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CanSnapshot")

	return nil
}

// GetSnapshot gets a snapshot. To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *SANStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get the volume (uses InternalID if available for faster lookup)
	volumeExists, volume, err := d.API.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		return nil, nil
	}

	// Get the specific snapshot directly (efficient - no need to list all snapshots)
	snapshot, err := d.API.SnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, nil // Snapshot not found
		}
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}

	// Check if snapshot is ready
	if snapshot.State != api.SnapshotStateReady {
		return nil, nil
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.Format(time.RFC3339),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume.
func (d *SANStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "SANStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	// Get volume (uses InternalID if available for faster lookup)
	volume, err := d.API.Volume(ctx, volConfig)
	if err != nil {
		return nil, err
	}

	snapshots, err := d.API.SnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Snapshot, 0, len(*snapshots))
	for _, snap := range *snapshots {
		result = append(result, &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				InternalName:       snap.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   snap.Created.Format(time.RFC3339),
			SizeBytes: 0,
			State:     storage.SnapshotStateOnline,
		})
	}

	return result, nil
}

// CreateFollowup is called after volume creation and sets the access info in the volume config.
func (d *SANStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method": "CreateFollowup",
		"Type":   "SANStorageDriver",
		"name":   volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

	return nil
}

// GetVolumeForImport queries the storage backend for all relevant info about a single volume.
func (d *SANStorageDriver) GetVolumeForImport(ctx context.Context, volumeName string) (*storage.VolumeExternal, error) {
	// Discover GCNV resources
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	volume, err := d.API.VolumeByNameOrID(ctx, volumeName)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volume), nil
}

// List returns the names of all volumes managed by this driver instance.
func (d *SANStorageDriver) List(ctx context.Context) ([]string, error) {
	fields := LogFields{"Method": "List", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> List")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< List")

	// Update resource cache as needed
	if err := d.API.RefreshGCNVResources(ctx); err != nil {
		return nil, fmt.Errorf("could not refresh GCNV resources: %v", err)
	}

	volumes, err := d.API.Volumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	volumeNames := make([]string, 0)

	for _, volume := range *volumes {
		// Filter out non-iSCSI volumes (this driver only manages iSCSI volumes)
		isISCSI := false
		for _, proto := range volume.ProtocolTypes {
			if proto == api.ProtocolTypeISCSI {
				isISCSI = true
				break
			}
		}
		if !isISCSI {
			continue
		}

		// Filter out volumes in an unavailable state
		switch volume.State {
		case api.VolumeStateDeleting, api.VolumeStateError, api.VolumeStateDisabled:
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
