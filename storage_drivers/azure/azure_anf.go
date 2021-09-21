// Copyright 2021 NetApp, Inc. All Rights Reserved.

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
	"github.com/netapp/trident/storage_drivers/azure/sdk"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes    = 1000000000   // 1 GB
	MinimumANFVolumeSizeBytes = 107374182400 // 100 GiB

	defaultNfsMountOptions = "-o nfsvers=3"
	defaultSnapshotDir     = "false"
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"
	defaultVolumeSizeStr   = "107374182400"

	// Constants for internal pool attributes
	Cookie         = "cookie"
	Size           = "size"
	ServiceLevel   = "serviceLevel"
	SnapshotDir    = "snapshotDir"
	ExportRule     = "exportRule"
	Location       = "location"
	VirtualNetwork = "virtualNetwork"
	Subnet         = "subnet"
	CapacityPools  = "capacityPools"

	nfsVersion3  = "3"
	nfsVersion4  = "4"
	nfsVersion41 = "4.1"
)

var (
	supportedNFSVersions = []string{nfsVersion3, nfsVersion4, nfsVersion41}
)

// NFSStorageDriver is for storage provisioning using Azure NetApp Files service in Azure
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.AzureNFSStorageDriverConfig
	telemetry           *Telemetry
	SDK                 *sdk.Client
	tokenRegexp         *regexp.Regexp
	pools               map[string]storage.Pool
	volumeCreateTimeout time.Duration
}

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}

func (d *NFSStorageDriver) GetConfig() *drivers.AzureNFSStorageDriverConfig {
	return &d.Config
}

func (d *NFSStorageDriver) GetSDK() *sdk.Client {
	return d.SDK
}

func (d *NFSStorageDriver) getTelemetry() *Telemetry {
	return d.telemetry
}

// Name returns the name of this driver
func (d *NFSStorageDriver) Name() string {
	return drivers.AzureNFSStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance
func (d *NFSStorageDriver) defaultBackendName() string {
	id := utils.RandomString(6)
	if len(d.Config.ClientID) > 5 {
		id = d.Config.ClientID[0:5]
	}
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), id)
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
		return fmt.Errorf("volume name '%s' is not allowed; it must be 16-36 characters long, "+
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
		return sdk.VolumeCreateTimeout
	}
}

// defaultTimeout controls the driver timeout for most API operations.
func (d *NFSStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return sdk.DefaultTimeout
	}
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
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{15,35}$`)

	// Parse the config
	config, err := d.initializeAzureConfig(ctx, configJSON, commonConfig, backendSecret)
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

	if d.SDK, err = d.initializeAzureSDKClient(ctx, &d.Config); err != nil {
		return fmt.Errorf("error initializing %s SDK client. %v", d.Name(), err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

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

	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
	}

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

	d.SDK.Terminate()

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NFSStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AzureNFSStorageDriverConfig,
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
		config.Size = defaultVolumeSizeStr
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
		"ServiceLevel":    config.ServiceLevel,
		"NfsMountOptions": config.NfsMountOptions,
		"SnapshotDir":     config.SnapshotDir,
		"LimitVolumeSize": config.LimitVolumeSize,
		"ExportRule":      config.ExportRule,
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
		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)

		if d.Config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes()[Size] = d.Config.Size
		pool.InternalAttributes()[ServiceLevel] = strings.Title(d.Config.ServiceLevel)
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[Location] = d.Config.Location
		pool.InternalAttributes()[VirtualNetwork] = d.Config.VirtualNetwork
		pool.InternalAttributes()[Subnet] = d.Config.Subnet
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

			location := d.Config.Location
			if vpool.Location != "" {
				location = vpool.Location
			}

			size := d.Config.Size
			if vpool.Size != "" {
				size = vpool.Size
			}

			supportedTopologies := d.Config.SupportedTopologies
			if vpool.SupportedTopologies != nil {
				supportedTopologies = vpool.SupportedTopologies
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
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[ServiceLevel] = strings.Title(serviceLevel)
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[Location] = location
			pool.InternalAttributes()[VirtualNetwork] = vnet
			pool.InternalAttributes()[Subnet] = subnet
			pool.InternalAttributes()[CapacityPools] = strings.Join(capacityPools, ",")

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}

	return nil
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
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		err := config.InjectSecrets(backendSecret)
		if err != nil {
			return nil, fmt.Errorf("could not inject backend secret; err: %v", err)
		}
	}

	return config, nil
}

// initializeAzureSDKClient returns an Azure SDK client.
func (d *NFSStorageDriver) initializeAzureSDKClient(
	ctx context.Context, config *drivers.AzureNFSStorageDriverConfig,
) (*sdk.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAzureSDKClient", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAzureSDKClient")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAzureSDKClient")
	}

	client := sdk.NewDriver(sdk.ClientConfig{
		SubscriptionID:  config.SubscriptionID,
		TenantID:        config.TenantID,
		ClientID:        config.ClientID,
		ClientSecret:    config.ClientSecret,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	if err := client.SDKClient.Authenticate(); err != nil {
		return nil, err
	}

	// Vpools should already be set up by now
	if err := client.Init(ctx, d.pools); err != nil {
		return nil, err
	}

	return client, nil
}

// validate ensures the driver configuration and execution environment are valid and working
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

	var err error
	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level (it is allowed to be blank)
		switch pool.InternalAttributes()[ServiceLevel] {
		case sdk.ServiceLevelStandard, sdk.ServiceLevelPremium, sdk.ServiceLevelUltra, "":
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
				return fmt.Errorf("invalid value for snapshotDir in pool %s: %v", poolName, err)
			}
		}

		// Validate default size
		if _, err = utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		}

		if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, sdk.MaxLabelLength); err != nil {
			return fmt.Errorf("invalid value for label in pool %s: %v", poolName, err)
		}

		// Validate that the Capacity Pools values are valid
		if pool.InternalAttributes()[CapacityPools] != "" {
			capacityPoolList := utils.SplitString(ctx, pool.InternalAttributes()[CapacityPools], ",")
			discoveredCapacityPoolNames := d.SDK.GetCapacityPoolNames()

			for _, capacityPool := range capacityPoolList {
				if !utils.SliceContainsString(discoveredCapacityPoolNames, capacityPool) {
					return fmt.Errorf("invalid value for capacity pool %s in pool %s", capacityPool, poolName)
				}
			}
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
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s: %v", name, err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == sdk.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", sdk.StateCreating, sdk.StateAvailable))
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
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size])
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if checkMinVolumeSizeError := drivers.CheckMinVolumeSize(sizeBytes,
		MinimumVolumeSizeBytes); checkMinVolumeSizeError != nil {
		return checkMinVolumeSizeError
	}

	// TODO: remove this code once ANF can handle smaller volumes
	if sizeBytes < MinimumANFVolumeSizeBytes {

		Logc(ctx).WithFields(log.Fields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (100 GB).")

		sizeBytes = MinimumANFVolumeSizeBytes
		volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

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
		return fmt.Errorf("invalid value for snapshotDir: %v", err)
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
		protocolTypes = []string{sdk.ProtocolTypeNFSv3}
	case nfsVersion4:
		fallthrough
	case nfsVersion41:
		nfsV41Access = true
		protocolTypes = []string{sdk.ProtocolTypeNFSv41}
	}

	apiExportRule := sdk.ExportRule{
		AllowedClients: pool.InternalAttributes()[ExportRule],
		Cifs:           cifsAccess,
		Nfsv3:          nfsV3Access,
		Nfsv41:         nfsV41Access,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := sdk.ExportPolicy{
		Rules: []sdk.ExportRule{apiExportRule},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = d.getTelemetryLabels(ctx)

	poolLabels, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, sdk.MaxLabelLength)
	if err != nil {
		return err
	}
	labels[storage.ProvisioningLabelTag] = poolLabels

	Logc(ctx).WithFields(log.Fields{
		"creationToken": name,
		"size":          sizeBytes,
		"serviceLevel":  serviceLevel,
		"snapshotDir":   snapshotDirBool,
		"protocolTypes": protocolTypes,
		"exportPolicy":  fmt.Sprintf("%+v", exportPolicy),
	}).Debug("Creating volume.")

	createRequest := &sdk.FilesystemCreateRequest{
		Name:              volConfig.Name,
		Location:          pool.InternalAttributes()[Location],
		VirtualNetwork:    pool.InternalAttributes()[VirtualNetwork],
		Subnet:            pool.InternalAttributes()[Subnet],
		CreationToken:     name,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		PoolID:            storagePool.Name(),
		ProtocolTypes:     protocolTypes,
		QuotaInBytes:      int64(sizeBytes),
		ServiceLevel:      serviceLevel,
		SnapshotDirectory: snapshotDirBool,
	}

	// Create the volume
	volume, err := d.SDK.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, volume, name)
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NFSStorageDriver) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool,
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
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s: %v", name, err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == sdk.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", sdk.StateCreating, sdk.StateAvailable))
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get the source volume
	sourceVolume, err := d.SDK.GetVolumeByCreationToken(ctx, source)
	if err != nil {
		return fmt.Errorf("could not find source volume: %v", err)
	}

	// Check if storage pool's list of capacity pools (if any) contains source volume's capacity pool
	if storagePool.Name() != "" {
		if !containsCpool(ctx, storagePool.InternalAttributes()[CapacityPools], sourceVolume.CapacityPoolName) {
			return fmt.Errorf("source volume's capacity pool '%s' not part of storage pool '%s'",
				sourceVolume.CapacityPoolName, storagePool.Name())
		}
	} else {
		if err = d.ensureVolPartOfValidCpool(ctx, sourceVolume.CapacityPoolName); err != nil {
			return err
		}
	}

	var sourceSnapshot *sdk.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.SDK.GetSnapshotForVolume(ctx, sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.ProvisioningState != sdk.StateAvailable {
			return fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.ProvisioningState, sdk.StateAvailable)
		}

		Logc(ctx).WithFields(log.Fields{
			"snapshot": snapshot,
			"source":   sourceVolume.Name,
		}).Debug("Found source snapshot.")

	} else {

		// No source snapshot specified, so create one
		snapName := time.Now().UTC().Format(storage.SnapshotNameFormat)
		snapshotCreateRequest := &sdk.SnapshotCreateRequest{
			FileSystemID: sourceVolume.FileSystemID,
			Volume:       sourceVolume,
			Name:         snapName,
			Location:     sourceVolume.Location,
		}

		Logc(ctx).WithFields(log.Fields{
			"snapshot": snapName,
			"source":   sourceVolume.Name,
		}).Debug("Creating source snapshot.")

		sourceSnapshot, err = d.SDK.CreateSnapshot(ctx, snapshotCreateRequest)
		if err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.SDK.WaitForSnapshotState(
			ctx, sourceSnapshot, sourceVolume, sdk.StateAvailable, []string{sdk.StateError}, sdk.SnapshotTimeout)
		if err != nil {
			return err
		}

		// Re-fetch the snapshot to populate the properties after create has completed
		sourceSnapshot, err = d.SDK.GetSnapshotForVolume(ctx, sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not retrieve newly-created snapshot")
		}

		Logc(ctx).WithFields(log.Fields{
			"snapshot": sourceSnapshot.Name,
			"source":   sourceVolume.Name,
		}).Debug("Created source snapshot.")
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshot.Name,
	}).Debug("Cloning volume.")

	cookie, err := d.SDK.GetCookieByCapacityPoolName(sourceVolume.CapacityPoolName)
	if err != nil {
		return fmt.Errorf("couldn't find cookie for volume %v", name)
	}

	var labels map[string]string
	labels = d.updateTelemetryLabels(ctx, sourceVolume)

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
		})
		poolLabels, err := storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, sdk.MaxLabelLength)
		if err != nil {
			return err
		}

		labels[storage.ProvisioningLabelTag] = poolLabels
	}

	createRequest := &sdk.FilesystemCreateRequest{
		Name:              volConfig.Name,
		Location:          sourceVolume.Location,
		CapacityPool:      sourceVolume.CapacityPoolName, // critical value for clone path
		Subnet:            sourceVolume.Subnet,
		CreationToken:     name,
		ExportPolicy:      sourceVolume.ExportPolicy,
		Labels:            labels,
		PoolID:            *cookie.StoragePoolName,
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		QuotaInBytes:      sourceVolume.QuotaInBytes,
		ServiceLevel:      sourceVolume.ServiceLevel,
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SnapshotID:        sourceSnapshot.SnapshotID,
	}

	// Clone the volume
	clone, err := d.SDK.CreateVolume(ctx, createRequest)
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	err = d.ensureVolPartOfValidCpool(ctx, volume.CapacityPoolName)
	if err != nil {
		return err
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	Logc(ctx).WithFields(log.Fields{
		"creationToken": volume.CreationToken,
		"managed":       !volConfig.ImportNotManaged,
		"state":         volume.ProvisioningState,
		"capacityPool":  volume.CapacityPoolName,
		"sizeBytes":     volume.QuotaInBytes,
	}).Debug("Found volume to import.")

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, volume.Labels[storage.ProvisioningLabelTag]) {
			volume.Labels[storage.ProvisioningLabelTag] = ""
		}

		if _, err := d.SDK.RelabelVolume(ctx, volume, d.updateTelemetryLabels(ctx, volume)); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, relabel failed: %v", err)
			return fmt.Errorf("could not import volume %s, relabel failed: %v", originalName, err)
		}
		_, err := d.SDK.WaitForVolumeState(
			ctx, volume, sdk.StateAvailable, []string{sdk.StateError}, d.defaultTimeout())
		if err != nil {
			return fmt.Errorf("could not import volume %s: %v", originalName, err)
		}
	}

	// The ANF creation token cannot be changed, so use it as the internal name
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
	// ANF volume when importing, so do nothing here lest we set the volume name incorrectly
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

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) updateTelemetryLabels(ctx context.Context, volume *sdk.FileSystem) map[string]string {

	newLabels := volume.Labels
	newLabels[drivers.TridentLabelTag] = d.getTelemetryLabels(ctx)
	return newLabels
}

// waitForVolumeCreate waits for volume creation to complete by reaching the Available state.  If the
// volume reaches a terminal state (Error), the volume is deleted.  If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
func (d *NFSStorageDriver) waitForVolumeCreate(ctx context.Context, volume *sdk.FileSystem, volumeName string) error {

	state, err := d.SDK.WaitForVolumeState(
		ctx, volume, sdk.StateAvailable, []string{sdk.StateError}, d.volumeCreateTimeout)
	if err != nil {

		// Don't leave an ANF volume laying around in error state
		if state == sdk.StateCreating {
			Logc(ctx).WithFields(log.Fields{
				"volume": volumeName,
			}).Debug("Volume is in creating state.")
			return utils.VolumeCreatingError(err.Error())
		}

		// Don't leave an ANF volume in a non-transitional state laying around in error state
		if !sdk.IsTransitionalState(ctx, state) {
			errDelete := d.SDK.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDelete)
			}
		} else {
			// Wait for deletion to complete
			_, errDeleteWait := d.SDK.WaitForVolumeState(
				ctx, volume, sdk.StateDeleted, []string{sdk.StateError}, d.defaultTimeout())
			if errDeleteWait != nil {
				Logc(ctx).WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDeleteWait)
			}
		}
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
	// 'name' is in fact 'creationToken' here.
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s: %v", name, err)
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.ProvisioningState == sdk.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.SDK.WaitForVolumeState(
			ctx, extantVolume, sdk.StateDeleted, []string{sdk.StateError}, d.volumeCreateTimeout)
		return err
	}

	// Delete the volume
	if err = d.SDK.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	// Wait for deletion to complete
	_, err = d.SDK.WaitForVolumeState(ctx, extantVolume, sdk.StateDeleted, []string{sdk.StateError}, d.defaultTimeout())
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
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
func (d *NFSStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig) error {
	return nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
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

	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s: %v", creationToken, err)
	}
	if !volumeExists {
		// The ANF volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil, nil
	}

	snapshots, err := d.SDK.GetSnapshotsForVolume(ctx, extantVolume)
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

// GetSnapshots returns the list of snapshots associated with the specified volume
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.SDK.GetSnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, snapshot := range *snapshots {

		// Filter out snapshots in an unavailable state
		if snapshot.ProvisioningState != sdk.StateAvailable {
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
	volumeExists, sourceVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s: %v", internalVolName, err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Construct a snapshot request
	snapshotCreateRequest := &sdk.SnapshotCreateRequest{
		FileSystemID: sourceVolume.FileSystemID,
		Volume:       sourceVolume,
		Name:         internalSnapName,
		Location:     sourceVolume.Location,
	}

	snapshot, err := d.SDK.CreateSnapshot(ctx, snapshotCreateRequest)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.SDK.WaitForSnapshotState(
		ctx, snapshot, sourceVolume, sdk.StateAvailable, []string{sdk.StateError}, sdk.SnapshotTimeout)
	if err != nil {
		return nil, err
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat),
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.SDK.GetSnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	return d.SDK.RestoreSnapshot(ctx, volume, snapshot)
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

	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s: %v", creationToken, err)
	}
	if !volumeExists {
		// The ANF volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil
	}

	snapshot, err := d.SDK.GetSnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	if err = d.SDK.DeleteSnapshot(ctx, extantVolume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	if err := d.SDK.WaitForSnapshotState(
		ctx, snapshot, extantVolume, sdk.StateDeleted, []string{sdk.StateError}, sdk.SnapshotTimeout,
	); err != nil {
		return err
	}

	return nil
}

// List returns the list of volumes associated with this tenant
func (d *NFSStorageDriver) List(ctx context.Context) ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> List")
		defer Logc(ctx).WithFields(fields).Debug("<<<< List")
	}

	volumes, err := d.SDK.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	var volumeNames []string

	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.ProvisioningState {
		case sdk.StateDeleting, sdk.StateDeleted, sdk.StateError:
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("could not get volume %s: %v", name, err)
	}

	return d.ensureVolPartOfValidCpool(ctx, volume.CapacityPoolName)
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// If the volume state isn't Available, return an error
	if volume.ProvisioningState != sdk.StateAvailable {
		return fmt.Errorf("volume %s state is %s, not available", creationToken, volume.ProvisioningState)
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
	_, err = d.SDK.ResizeVolume(ctx, volume, int64(sizeBytes))
	if err != nil {
		return err
	}

	// Wait for resize operation to complete
	_, err = d.SDK.WaitForVolumeState(ctx, volume, sdk.StateAvailable, []string{sdk.StateError}, d.defaultTimeout())
	if err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
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

func (d *NFSStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NFSStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return []string{}
}

func (d *NFSStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// Cloud volumes have strict limits on volume mount paths, so for cloud
		// infrastructure like Trident, the simplest approach is to generate a
		// UUID-based name with a prefix that won't exceed the 36-character limit.
		return "anf-" + strings.Replace(uuid.New().String(), "-", "", -1)
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

	volume, err := d.SDK.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Ensure volume is in a good state
	if volume.ProvisioningState == sdk.StateError {
		return fmt.Errorf("volume %s is in %s state: %s", creationToken, sdk.StateError, volume.ProvisioningState)
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

func (d *NFSStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NFSStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AzureConfig = &d.Config
}

func (d *NFSStorageDriver) GetExternalConfig(ctx context.Context) interface{} {

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AzureNFSStorageDriverConfig
	drivers.Clone(ctx, d.Config, &cloneConfig)
	cloneConfig.ClientSecret = drivers.REDACTED // redact the Secret
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: drivers.REDACTED,
		drivers.KeyType: drivers.REDACTED,
	} // redact the credentials
	return cloneConfig
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NFSStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.SDK.GetVolumeByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volumeAttrs), nil
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

	// Get all volumes
	volumes, err := d.SDK.GetVolumes(ctx)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	prefix := *d.Config.StoragePrefix

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.ProvisioningState {
		case sdk.StateDeleting, sdk.StateDeleted, sdk.StateError:
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
func (d *NFSStorageDriver) getVolumeExternal(volumeAttrs *sdk.FileSystem) *storage.VolumeExternal {

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
		ServiceLevel:    volumeAttrs.ServiceLevel,
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}

// String implements stringer interface for the NFSStorageDriver driver
func (d NFSStorageDriver) String() string {
	return drivers.ToString(&d, []string{"SDK"}, d.GetExternalConfig(context.Background()))
}

// GoString implements GoStringer interface for the NFSStorageDriver driver
func (d NFSStorageDriver) GoString() string {
	return d.String()
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
	matched, err := regexp.MatchString(`[^a-zA-Z-]+`, storagePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if matched {
		return fmt.Errorf("storage prefix may only contain letters and hyphens")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d NFSStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d NFSStorageDriver) ensureVolPartOfValidCpool(ctx context.Context, volumeCpoolName string) error {
	// If there is a vpool without any capacityPools flag, then skip this check
	// else ensure cpool name must exist in one of the vpool's cpool lists
	var volInValidCpool bool
	for _, vpool := range d.pools {
		if containsCpool(ctx, vpool.InternalAttributes()[CapacityPools], volumeCpoolName) {
			volInValidCpool = true
			break
		}
	}

	if !volInValidCpool {
		return utils.NotFoundError(
			fmt.Sprintf("volume is part of another capacity pool not referenced by the backend %s", d.BackendName()))
	}

	return nil
}

func containsCpool(ctx context.Context, cpoolListString, cpoolName string) bool {
	cpoolList := utils.SplitString(ctx, cpoolListString, ",")
	if len(cpoolList) == 0 {
		return true
	} else if utils.SliceContainsString(cpoolList, cpoolName) {
		return true
	}

	return false
}
