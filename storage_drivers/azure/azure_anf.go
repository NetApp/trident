// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azure

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
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes    = uint64(1000000000)   // 1 GB
	MinimumANFVolumeSizeBytes = uint64(107374182400) // 100 GiB

	defaultUnixPermissions = "" // TODO (cknight): change to "0777" when whitelisted permissions feature reaches GA
	defaultNfsMountOptions = "nfsvers=3"
	defaultSnapshotDir     = "false"
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"
	defaultVolumeSizeStr   = "107374182400"

	// Constants for internal pool attributes

	Size            = "size"
	UnixPermissions = "unixPermissions"
	ServiceLevel    = "serviceLevel"
	SnapshotDir     = "snapshotDir"
	ExportRule      = "exportRule"
	VirtualNetwork  = "virtualNetwork"
	Subnet          = "subnet"
	ResourceGroups  = "resourceGroups"
	NetappAccounts  = "netappAccounts"
	CapacityPools   = "capacityPools"
	FilePoolVolumes = "filePoolVolumes"

	nfsVersion3  = "3"
	nfsVersion4  = "4"
	nfsVersion41 = "4.1"
)

var (
	supportedNFSVersions = []string{nfsVersion3, nfsVersion4, nfsVersion41}

	storagePrefixRegex       = regexp.MustCompile(`^$|^[a-zA-Z][a-zA-Z-]*$`)
	volumeNameRegex          = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-_]{0,63}$`)
	volumeCreationTokenRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	csiRegex                 = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// NFSStorageDriver is for storage provisioning using the Azure NetApp Files service.
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.AzureNFSStorageDriverConfig
	telemetry           *Telemetry
	SDK                 api.Azure
	pools               map[string]storage.Pool
	volumeCreateTimeout time.Duration
}

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}

// Name returns the name of this driver.
func (d *NFSStorageDriver) Name() string {
	return drivers.AzureNFSStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance.
func (d *NFSStorageDriver) defaultBackendName() string {
	id := utils.RandomString(6)
	if len(d.Config.ClientID) > 5 {
		id = d.Config.ClientID[0:5]
	}
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), id)
}

// BackendName returns the name of the backend managed by this driver instance.
func (d *NFSStorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		// Use the old naming scheme if no name is specified
		return d.defaultBackendName()
	}
}

// poolName constructs the name of the pool reported by this driver instance.
func (d *NFSStorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// validateVolumeName checks that the name of a new volume matches the requirements of an ANF volume name.
func (d *NFSStorageDriver) validateVolumeName(name string) error {
	if !volumeNameRegex.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be 1-64 characters long, "+
			"begin with a letter, and contain only letters, digits, hyphens, and underscores", name)
	}
	return nil
}

// validateCreationToken checks that the creation token of a new volume matches the requirements of a creation token.
func (d *NFSStorageDriver) validateCreationToken(name string) error {
	if !volumeCreationTokenRegex.MatchString(name) {
		return fmt.Errorf("volume internal name '%s' is not allowed; it must be 1-80 characters long, "+
			"begin with a letter, and contain only letters, digits, and hyphens", name)
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

// defaultTimeout controls the driver timeout for most workflows.
func (d *NFSStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return api.DefaultTimeout
	}
}

// Initialize initializes this driver from the provided config.
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

	// Parse the config
	config, err := d.initializeAzureConfig(ctx, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
	}
	d.Config = *config

	d.populateConfigurationDefaults(ctx, &d.Config)
	d.initializeStoragePools(ctx)
	d.initializeTelemetry(ctx, backendUUID)

	if err = d.initializeAzureSDKClient(ctx, &d.Config); err != nil {
		return fmt.Errorf("error initializing %s SDK client. %v", d.Name(), err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		if i, parseErr := strconv.ParseUint(d.Config.VolumeCreateTimeout, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.VolumeCreateTimeout).WithError(parseErr).Error(
				"Invalid volume create timeout period.")
			return parseErr
		} else {
			volumeCreateTimeout = time.Duration(i) * time.Second
		}
	}
	d.volumeCreateTimeout = volumeCreateTimeout

	Logc(ctx).WithFields(log.Fields{
		"StoragePrefix":              *config.StoragePrefix,
		"Size":                       config.Size,
		"ServiceLevel":               config.ServiceLevel,
		"NfsMountOptions":            config.NfsMountOptions,
		"LimitVolumeSize":            config.LimitVolumeSize,
		"ExportRule":                 config.ExportRule,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	})

	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated).
func (d *NFSStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded.
func (d *NFSStorageDriver) Terminate(ctx context.Context, _ string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config.
func (d *NFSStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AzureNFSStorageDriverConfig,
) {

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
		config.Size = defaultVolumeSizeStr
	}

	if config.UnixPermissions == "" {
		config.UnixPermissions = defaultUnixPermissions
	}

	if config.NfsMountOptions == "" {
		config.NfsMountOptions = defaultNfsMountOptions
	}

	if config.SnapshotDir == "" {
		config.SnapshotDir = defaultSnapshotDir
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	if config.ExportRule == "" {
		config.ExportRule = defaultExportRule
	}

	Logc(ctx).WithFields(log.Fields{
		"StoragePrefix":   *config.StoragePrefix,
		"Size":            config.Size,
		"UnixPermissions": config.UnixPermissions,
		"ServiceLevel":    config.ServiceLevel,
		"NfsMountOptions": config.NfsMountOptions,
		"SnapshotDir":     config.SnapshotDir,
		"LimitVolumeSize": config.LimitVolumeSize,
		"ExportRule":      config.ExportRule,
	}).Debugf("Configuration defaults")

	return
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
		pool.InternalAttributes()[UnixPermissions] = d.Config.UnixPermissions
		pool.InternalAttributes()[ServiceLevel] = strings.Title(d.Config.ServiceLevel)
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[VirtualNetwork] = d.Config.VirtualNetwork
		pool.InternalAttributes()[Subnet] = d.Config.Subnet
		pool.InternalAttributes()[ResourceGroups] = strings.Join(d.Config.ResourceGroups, ",")
		pool.InternalAttributes()[NetappAccounts] = strings.Join(d.Config.NetappAccounts, ",")
		pool.InternalAttributes()[CapacityPools] = strings.Join(d.Config.CapacityPools, ",")

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

			resourceGroups := d.Config.ResourceGroups
			if vpool.ResourceGroups != nil {
				resourceGroups = vpool.ResourceGroups
			}

			netappAccounts := d.Config.NetappAccounts
			if vpool.NetappAccounts != nil {
				netappAccounts = vpool.NetappAccounts
			}

			capacityPools := d.Config.CapacityPools
			if vpool.CapacityPools != nil {
				capacityPools = vpool.CapacityPools
			}

			serviceLevel := d.Config.ServiceLevel
			if vpool.ServiceLevel != "" {
				serviceLevel = vpool.ServiceLevel
			}

			snapshotDir := d.Config.SnapshotDir
			if vpool.SnapshotDir != "" {
				snapshotDir = vpool.SnapshotDir
			}

			exportRule := d.Config.ExportRule
			if vpool.ExportRule != "" {
				exportRule = vpool.ExportRule
			}

			vnet := d.Config.VirtualNetwork
			if vpool.VirtualNetwork != "" {
				vnet = vpool.VirtualNetwork
			}

			subnet := d.Config.Subnet
			if vpool.Subnet != "" {
				subnet = vpool.Subnet
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
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[ServiceLevel] = strings.Title(serviceLevel)
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[VirtualNetwork] = vnet
			pool.InternalAttributes()[Subnet] = subnet
			pool.InternalAttributes()[ResourceGroups] = strings.Join(resourceGroups, ",")
			pool.InternalAttributes()[NetappAccounts] = strings.Join(netappAccounts, ",")
			pool.InternalAttributes()[CapacityPools] = strings.Join(capacityPools, ",")

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}

	return
}

// initializeTelemetry assembles all the telemetry data to be used as volume labels.
func (d *NFSStorageDriver) initializeTelemetry(_ context.Context, backendUUID string) {

	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
	}
}

// initializeAzureConfig parses the Azure config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeAzureConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.AzureNFSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAzureConfig", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAzureConfig")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAzureConfig")
	}

	config := &drivers.AzureNFSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AzureNFSStorageDriverConfig object
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration; %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		err := config.InjectSecrets(backendSecret)
		if err != nil {
			return nil, fmt.Errorf("could not inject backend secret; %v", err)
		}
	}

	return config, nil
}

// initializeAzureSDKClient creates and initializes an Azure SDK client.
func (d *NFSStorageDriver) initializeAzureSDKClient(
	ctx context.Context, config *drivers.AzureNFSStorageDriverConfig,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAzureSDKClient", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAzureSDKClient")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAzureSDKClient")
	}

	sdkTimeout := api.DefaultSDKTimeout
	if config.SDKTimeout != "" {
		if i, parseErr := strconv.ParseUint(d.Config.SDKTimeout, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.SDKTimeout).WithError(parseErr).Error(
				"Invalid value for SDK timeout.")
			return parseErr
		} else {
			sdkTimeout = time.Duration(i) * time.Second
		}
	}

	maxCacheAge := api.DefaultMaxCacheAge
	if config.MaxCacheAge != "" {
		if i, parseErr := strconv.ParseUint(d.Config.MaxCacheAge, 10, 64); parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.MaxCacheAge).WithError(parseErr).Error(
				"Invalid value for max cache age.")
			return parseErr
		} else {
			maxCacheAge = time.Duration(i) * time.Second
		}
	}

	client, err := api.NewDriver(api.ClientConfig{
		SubscriptionID:  config.SubscriptionID,
		TenantID:        config.TenantID,
		ClientID:        config.ClientID,
		ClientSecret:    config.ClientSecret,
		Location:        config.Location,
		DebugTraceFlags: config.DebugTraceFlags,
		SDKTimeout:      sdkTimeout,
		MaxCacheAge:     maxCacheAge,
	})
	if err != nil {
		return err
	}

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.SDK == nil {
		d.SDK = client
	}

	// The storage pools should already be set up by this point. We register the pools with the
	// API layer to enable matching of storage pools with discovered ANF resources.
	return d.SDK.Init(ctx, d.pools)
}

// validate ensures the driver configuration and execution environment are valid and working.
func (d *NFSStorageDriver) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level (it is allowed to be blank)
		serviceLevel := pool.InternalAttributes()[ServiceLevel]
		switch serviceLevel {
		case api.ServiceLevelStandard, api.ServiceLevelPremium, api.ServiceLevelUltra, "":
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
			_, err := strconv.ParseBool(pool.InternalAttributes()[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s; %v", poolName, err)
			}
		}

		// Validate unix permissions
		if pool.InternalAttributes()[UnixPermissions] != "" {
			err := utils.ValidateOctalUnixPermissions(pool.InternalAttributes()[UnixPermissions])
			if err != nil {
				return fmt.Errorf("invalid value for unixPermissions in pool %s; %v", poolName, err)
			}
		}

		// Validate default size
		if _, err := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s; %v", poolName, err)
		}

		// Validate pool labels
		if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength); err != nil {
			return fmt.Errorf("invalid value for label in pool %s; %v", poolName, err)
		}
	}

	return nil
}

// Create creates a new volume.
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
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
	volumeExists, extantVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}

		Logc(ctx).WithFields(log.Fields{
			"name":  name,
			"state": extantVolume.ProvisioningState,
		}).Warning("Volume already exists.")

		return drivers.NewVolumeExistsError(name)
	}

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s; %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size; %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size])
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if err = drivers.CheckMinVolumeSize(sizeBytes, MinimumVolumeSizeBytes); err != nil {
		return err
	}

	if sizeBytes < MinimumANFVolumeSizeBytes {

		Logc(ctx).WithFields(log.Fields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (100 GB).")

		sizeBytes = MinimumANFVolumeSizeBytes
	}

	if _, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// We may have adjusted the size, so save it in the config
	volConfig.Size = strconv.FormatUint(sizeBytes, 10)

	// Take service level from volume config first (handles Docker case), then from pool
	serviceLevel := strings.Title(volConfig.ServiceLevel)
	if serviceLevel == "" {
		serviceLevel = pool.InternalAttributes()[ServiceLevel]
	}

	// Take snapshot directory from volume config first (handles Docker case), then from pool
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes()[SnapshotDir]
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir; %v", err)
	}

	// Take unix permissions from volume config first (handles Docker case & PVC annotations), then from pool
	unixPermissions := volConfig.UnixPermissions
	if unixPermissions == "" {
		unixPermissions = pool.InternalAttributes()[UnixPermissions]
	}

	// TODO (cknight): remove when preview permissions feature reaches GA
	if unixPermissions == "" && d.SDK.HasFeature(api.FeatureUnixPermissions) {
		unixPermissions = "0777"
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Determine protocol from mount options
	var protocolTypes []string
	var cifsAccess, nfsV3Access, nfsV41Access bool

	nfsVersion, err := utils.GetNFSVersionFromMountOptions(mountOptions, nfsVersion3, supportedNFSVersions)
	if err != nil {
		return err
	}
	switch nfsVersion {
	case nfsVersion3:
		nfsV3Access = true
		protocolTypes = []string{api.ProtocolTypeNFSv3}
	case nfsVersion4:
		fallthrough
	case nfsVersion41:
		nfsV41Access = true
		protocolTypes = []string{api.ProtocolTypeNFSv41}
	}

	apiExportRule := api.ExportRule{
		AllowedClients: pool.InternalAttributes()[ExportRule],
		Cifs:           cifsAccess,
		Nfsv3:          nfsV3Access,
		Nfsv41:         nfsV41Access,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = d.getTelemetryLabels(ctx)

	poolLabels, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	if err != nil {
		return err
	}
	labels[storage.ProvisioningLabelTag] = poolLabels

	// Find a capacity pool
	cPool := d.SDK.RandomCapacityPoolForStoragePool(ctx, pool, serviceLevel)
	if cPool == nil {
		return fmt.Errorf("no capacity pools found for storage pool %s", pool.Name())
	}

	// Find a subnet
	subnet := d.SDK.RandomSubnetForStoragePool(ctx, pool)
	if subnet == nil {
		return fmt.Errorf("no subnets found for storage pool %s", pool.Name())
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":   name,
		"size":            sizeBytes,
		"unixPermissions": unixPermissions,
		"serviceLevel":    serviceLevel,
		"snapshotDir":     snapshotDirBool,
		"protocolTypes":   protocolTypes,
		"exportPolicy":    fmt.Sprintf("%+v", exportPolicy),
	}).Debug("Creating volume.")

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:     cPool.ResourceGroup,
		NetAppAccount:     cPool.NetAppAccount,
		CapacityPool:      cPool.Name,
		Name:              volConfig.Name,
		SubnetID:          subnet.ID,
		CreationToken:     name,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProtocolTypes:     protocolTypes,
		QuotaInBytes:      int64(sizeBytes),
		SnapshotDirectory: snapshotDirBool,
		UnixPermissions:   unixPermissions,
	}

	// Create the volume
	volume, err := d.SDK.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Always save the ID so we can find the volume efficiently later
	volConfig.InternalID = volume.ID

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, volume)
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NFSStorageDriver) CreateClone(
	ctx context.Context, sourceVolConfig, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
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
	sourceVolume, err := d.SDK.Volume(ctx, sourceVolConfig)
	if err != nil {
		return fmt.Errorf("could not find source volume; %v", err)
	}

	// We always clone to the same capacity pool, so we can use the source volume's info to find
	// a pre-existing clone more efficiently.
	cloneID := api.CreateVolumeID(d.Config.SubscriptionID, sourceVolume.ResourceGroup, sourceVolume.NetAppAccount,
		sourceVolume.CapacityPool, cloneVolConfig.Name)

	// If the volume already exists, bail out
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByID(ctx, cloneID)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}
		return drivers.NewVolumeExistsError(name)
	}

	var sourceSnapshot *api.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.SDK.SnapshotForVolume(ctx, sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot; %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.ProvisioningState != api.StateAvailable {
			return fmt.Errorf("source snapshot state is %s, it must be %s",
				sourceSnapshot.ProvisioningState, api.StateAvailable)
		}

		Logc(ctx).WithFields(log.Fields{
			"snapshot": snapshot,
			"source":   sourceVolume.Name,
		}).Debug("Found source snapshot.")

	} else {

		// No source snapshot specified, so create one
		snapName := time.Now().UTC().Format(storage.SnapshotNameFormat)

		Logc(ctx).WithFields(log.Fields{
			"snapshot": snapName,
			"source":   sourceVolume.Name,
		}).Debug("Creating source snapshot.")

		sourceSnapshot, err = d.SDK.CreateSnapshot(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not create source snapshot; %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.SDK.WaitForSnapshotState(
			ctx, sourceSnapshot, sourceVolume, api.StateAvailable, []string{api.StateError}, api.SnapshotTimeout)
		if err != nil {
			return err
		}

		// Re-fetch the snapshot to populate the properties after create has completed
		sourceSnapshot, err = d.SDK.SnapshotForVolume(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not retrieve newly-created snapshot")
		}

		Logc(ctx).WithFields(log.Fields{
			"snapshot": sourceSnapshot.Name,
			"source":   sourceVolume.Name,
		}).Debug("Created source snapshot.")
	}

	var labels map[string]string
	labels = d.updateTelemetryLabels(ctx, sourceVolume)

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.Config.Labels),
		})
		poolLabels, err := storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
		if err != nil {
			return err
		}

		labels[storage.ProvisioningLabelTag] = poolLabels
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":   name,
		"sourceVolume":    sourceVolume.CreationToken,
		"sourceSnapshot":  sourceSnapshot.Name,
		"unixPermissions": sourceVolume.UnixPermissions,
	}).Debug("Cloning volume.")

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:     sourceVolume.ResourceGroup,
		NetAppAccount:     sourceVolume.NetAppAccount,
		CapacityPool:      sourceVolume.CapacityPool,
		Name:              cloneVolConfig.Name,
		SubnetID:          sourceVolume.SubnetID,
		CreationToken:     name,
		ExportPolicy:      sourceVolume.ExportPolicy,
		Labels:            labels,
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		QuotaInBytes:      sourceVolume.QuotaInBytes,
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SnapshotID:        sourceSnapshot.SnapshotID,
		UnixPermissions:   sourceVolume.UnixPermissions,
	}

	// Clone the volume
	clone, err := d.SDK.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Always save the ID so we can find the volume efficiently later
	cloneVolConfig.InternalID = clone.ID

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, clone)
}

// Import finds an existing volume and makes it available for containers.  If ImportNotManaged is false, the
// volume is fully brought under Trident's management.
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.VolumeByCreationToken(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", originalName, err)
	}

	// Ensure the volume may be imported by a capacity pool managed by this backend
	if err = d.SDK.EnsureVolumeInValidCapacityPool(ctx, volume); err != nil {
		return err
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	Logc(ctx).WithFields(log.Fields{
		"creationToken": volume.CreationToken,
		"managed":       !volConfig.ImportNotManaged,
		"state":         volume.ProvisioningState,
		"capacityPool":  volume.CapacityPool,
		"sizeBytes":     volume.QuotaInBytes,
	}).Debug("Found volume to import.")

	// Modify the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {

		// Update the volume labels
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, volume.Labels[storage.ProvisioningLabelTag]) {
			volume.Labels[storage.ProvisioningLabelTag] = ""
		}
		labels := d.updateTelemetryLabels(ctx, volume)

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
			if err = utils.ValidateOctalUnixPermissions(unixPermissions); err != nil {
				return fmt.Errorf("could not import volume %s; %v", originalName, err)
			}
		}

		if err = d.SDK.ModifyVolume(ctx, volume, labels, &unixPermissions); err != nil {
			Logc(ctx).WithField("originalName", originalName).WithError(err).Error(
				"Could not import volume, volume modify failed.")
			return fmt.Errorf("could not import volume %s, volume modify failed; %v", originalName, err)
		}

		Logc(ctx).WithFields(log.Fields{
			"name":            volume.Name,
			"creationToken":   volume.CreationToken,
			"labels":          labels,
			"unixPermissions": unixPermissions,
		}).Info("Volume modified.")

		if _, err = d.SDK.WaitForVolumeState(
			ctx, volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout()); err != nil {
			return fmt.Errorf("could not import volume %s; %v", originalName, err)
		}
	}

	// The ANF creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = originalName

	// Always save the ID so we can find the volume efficiently later
	volConfig.InternalID = volume.ID

	return nil
}

// Rename changes the name of a volume.  Not supported by this driver.
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
	// ANF volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

// getTelemetryLabels builds the standard telemetry labels that are set on each volume.
func (d *NFSStorageDriver) getTelemetryLabels(ctx context.Context) string {

	telemetry := map[string]Telemetry{drivers.TridentLabelTag: *d.telemetry}

	telemetryJSON, err := json.Marshal(telemetry)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal telemetry: %+v", telemetry)
	}

	return strings.ReplaceAll(string(telemetryJSON), " ", "")
}

// updateTelemetryLabels updates a volume's labels to include the standard telemetry labels.
func (d *NFSStorageDriver) updateTelemetryLabels(ctx context.Context, volume *api.FileSystem) map[string]string {

	if volume.Labels == nil {
		volume.Labels = make(map[string]string)
	}

	newLabels := volume.Labels
	newLabels[drivers.TridentLabelTag] = d.getTelemetryLabels(ctx)
	return newLabels
}

// waitForVolumeCreate waits for volume creation to complete by reaching the Available state.  If the
// volume reaches a terminal state (Error), the volume is deleted.  If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
func (d *NFSStorageDriver) waitForVolumeCreate(ctx context.Context, volume *api.FileSystem) error {

	state, err := d.SDK.WaitForVolumeState(
		ctx, volume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		logFields := log.Fields{"volume": volume.CreationToken}

		switch state {

		case api.StateAccepted, api.StateCreating:
			Logc(ctx).WithFields(logFields).Debugf("Volume is in %s state.", state)
			return utils.VolumeCreatingError(err.Error())

		case api.StateDeleting:
			// Wait for deletion to complete
			_, errDelete := d.SDK.WaitForVolumeState(
				ctx, volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
			}

		case api.StateError:
			// Delete a failed volume
			errDelete := d.SDK.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
			} else {
				Logc(ctx).WithField("volume", volume.Name).Info("Volume deleted.")
			}

		case api.StateMoving:
			fallthrough

		default:
			Logc(ctx).WithFields(logFields).Errorf("unexpected volume state %s found for volume", state)
		}
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// If volume doesn't exist, return success
	volumeExists, extantVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.ProvisioningState == api.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.SDK.WaitForVolumeState(
			ctx, extantVolume, api.StateDeleted, []string{api.StateError}, d.volumeCreateTimeout)
		return err
	}

	// Delete the volume
	if err = d.SDK.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	Logc(ctx).WithField("volume", extantVolume.Name).Info("Volume deleted.")

	// Wait for deletion to complete
	_, err = d.SDK.WaitForVolumeState(ctx, extantVolume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// Heal the ID on legacy volumes
	if volConfig.InternalID == "" {
		volConfig.InternalID = volume.ID
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = "/" + volume.CreationToken
	publishInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NFSStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NFSStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volumeExists, extantVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		// The ANF volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil, nil
	}

	// Get the snapshot
	snapshot, err := d.SDK.SnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}

	if snapshot.ProvisioningState != api.StateAvailable {
		return nil, fmt.Errorf("snapshot %s state is %s", internalSnapName, snapshot.ProvisioningState)
	}

	created := snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat)

	Logc(ctx).WithFields(log.Fields{
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s; %v", internalVolName, err)
	}

	snapshots, err := d.SDK.SnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, snapshot := range *snapshots {

		// Filter out snapshots in an unavailable state
		if snapshot.ProvisioningState != api.StateAvailable {
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
			Created:   snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat),
			SizeBytes: 0,
			State:     storage.SnapshotStateOnline,
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *NFSStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Check if volume exists
	volumeExists, sourceVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Create the snapshot
	snapshot, err := d.SDK.CreateSnapshot(ctx, sourceVolume, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot; %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.SDK.WaitForSnapshotState(
		ctx, snapshot, sourceVolume, api.StateAvailable, []string{api.StateError}, api.SnapshotTimeout)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Info("Snapshot created.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.  Not supported by this driver.
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

	return utils.UnsupportedError(fmt.Sprintf("restoring snapshots is not supported by backend type %s", d.Name()))
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *NFSStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volumeExists, extantVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", internalVolName, err)
	}
	if !volumeExists {
		// The ANF volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil
	}

	snapshot, err := d.SDK.SnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		// If the snapshot is already gone, return success
		if utils.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("unable to find snapshot %s; %v", internalSnapName, err)
	}

	if err = d.SDK.DeleteSnapshot(ctx, extantVolume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	return d.SDK.WaitForSnapshotState(
		ctx, snapshot, extantVolume, api.StateDeleted, []string{api.StateError}, api.SnapshotTimeout,
	)
}

// List returns the list of volumes associated with this backend.
func (d *NFSStorageDriver) List(ctx context.Context) ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> List")
		defer Logc(ctx).WithFields(fields).Debug("<<<< List")
	}

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	volumes, err := d.SDK.Volumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	volumeNames := make([]string, 0)

	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.ProvisioningState {
		case api.StateDeleting, api.StateDeleted, api.StateError:
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
func (d *NFSStorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	if _, err := d.SDK.VolumeByCreationToken(ctx, name); err != nil {
		return fmt.Errorf("could not get volume %s; %v", name, err)
	}

	return nil
}

// Resize increases a volume's quota.
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// Heal the ID on legacy volumes
	if volConfig.InternalID == "" {
		volConfig.InternalID = volume.ID
	}

	// If the volume state isn't Available, return an error
	if volume.ProvisioningState != api.StateAvailable {
		return fmt.Errorf("volume %s state is %s, not %s", name, volume.ProvisioningState, api.StateAvailable)
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
	if err = d.SDK.ResizeVolume(ctx, volume, int64(sizeBytes)); err != nil {
		return err
	}

	// Wait for resize operation to complete
	_, err = d.SDK.WaitForVolumeState(ctx, volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return fmt.Errorf("could not resize volume %s; %v", name, err)
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
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

// CreatePrepare is called prior to volume creation.  Currently its only role is to create the internal volume name.
func (d *NFSStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NFSStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return []string{}
}

// GetInternalVolumeName accepts the name of a volume being created and returns what the internal name
// should be, depending on backend requirements and Trident's operating context.
func (d *NFSStorageDriver) GetInternalVolumeName(ctx context.Context, name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else if csiRegex.MatchString(name) {
		// If the name is from CSI (i.e. contains a UUID), just use it as-is
		Logc(ctx).WithField("volumeInternal", name).Debug("Using volume name as internal name.")
		return name
	} else {
		// Cloud volumes have strict limits on volume mount paths, so for cloud
		// infrastructure like Trident, the simplest approach is to generate a
		// UUID-based name with a prefix that won't exceed the 36-character limit.
		return "anf-" + uuid.NewString()
	}
}

// CreateFollowup is called after volume creation and sets the access info in the volume config.
func (d *NFSStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "CreateFollowup",
			"Type":   "NFSStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateFollowup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateFollowup")
	}

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// Ensure volume is in a good state
	if volume.ProvisioningState != api.StateAvailable {
		return fmt.Errorf("volume %s is in %s state, not %s", name, volume.ProvisioningState, api.StateAvailable)
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volConfig.InternalName)
	}

	// Just use the first mount target found
	volConfig.AccessInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
	volConfig.AccessInfo.NfsPath = "/" + volume.CreationToken
	volConfig.FileSystem = ""

	return nil
}

// GetProtocol returns the protocol supported by this driver (File).
func (d *NFSStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

// StoreConfig adds this backend's config to the persistent config struct, as needed by Trident's persistence layer.
func (d *NFSStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AzureConfig = &d.Config
}

// GetExternalConfig returns returns a clone of this backend's config, sanitized for external consumption.
func (d *NFSStorageDriver) GetExternalConfig(ctx context.Context) interface{} {

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AzureNFSStorageDriverConfig
	drivers.Clone(ctx, d.Config, &cloneConfig)
	cloneConfig.ClientSecret = utils.REDACTED // redact the Secret
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

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	filesystem, err := d.SDK.VolumeByCreationToken(ctx, name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(filesystem), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NFSStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "GetVolumeExternalWrappers", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolumeExternalWrappers")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumeExternalWrappers")
	}

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all volumes
	volumes, err := d.SDK.Volumes(ctx)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	prefix := *d.Config.StoragePrefix

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.ProvisioningState {
		case api.StateDeleting, api.StateDeleted, api.StateError:
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
func (d *NFSStorageDriver) getVolumeExternal(volumeAttrs *api.FileSystem) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            volumeAttrs.Name,
		InternalName:    volumeAttrs.CreationToken,
		Size:            strconv.FormatInt(volumeAttrs.QuotaInBytes, 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     strconv.FormatBool(volumeAttrs.SnapshotDirectory),
		UnixPermissions: volumeAttrs.UnixPermissions,
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

// String implements stringer interface for the NFSStorageDriver driver.
func (d NFSStorageDriver) String() string {
	return utils.ToStringRedacted(&d, []string{"SDK"}, d.GetExternalConfig(context.Background()))
}

// GoString implements GoStringer interface for the NFSStorageDriver driver.
func (d NFSStorageDriver) GoString() string {
	return d.String()
}

// GetUpdateType returns a bitmap populated with updates to the driver.
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

// ReconcileNodeAccess updates a per-backend export policy to match the set of Kubernetes cluster
// nodes.  Not supported by this driver.
func (d *NFSStorageDriver) ReconcileNodeAccess(ctx context.Context, _ []*utils.Node, _ string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NFSStorageDriver",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	return nil
}

// validateStoragePrefix ensures the storage prefix is valid
func validateStoragePrefix(storagePrefix string) error {
	if !storagePrefixRegex.MatchString(storagePrefix) {
		return fmt.Errorf("storage prefix may only contain letters and hyphens and must begin with a letter")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d NFSStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}
