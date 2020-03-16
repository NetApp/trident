// Copyright 2020 NetApp, Inc. All Rights Reserved.

package azure

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	api "github.com/netapp/trident/storage_drivers/azure/sdk"

	"github.com/RoaringBitmap/roaring"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
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
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"
	defaultVolumeSizeStr   = "107374182400"

	// Constants for internal pool attributes
	Cookie         = "cookie"
	Size           = "size"
	ServiceLevel   = "serviceLevel"
	ExportRule     = "exportRule"
	Location       = "location"
	VirtualNetwork = "virtualNetwork"
	Subnet         = "subnet"
)

// NFSStorageDriver is for storage provisioning using Azure NetApp Files service in Azure
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.AzureNFSStorageDriverConfig
	SDK                 *sdk.Client
	tokenRegexp         *regexp.Regexp
	pools               map[string]*storage.Pool
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
	return &Telemetry{
		Telemetry: tridentconfig.OrchestratorTelemetry,
		Plugin:    d.Name(),
	}
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

// backendName returns the name of the backend managed by this driver instance
func (d *NFSStorageDriver) backendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		// Use the old naming scheme if no name is specified
		return d.defaultBackendName()
	}
}

// poolName constructs the name of the pool reported by this driver instance
func (d *NFSStorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.backendName(), strings.Replace(name, "-", "", -1))
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
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> Initialize")
		defer log.WithFields(fields).Debug("<<<< Initialize")
	}

	commonConfig.DriverContext = context
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{15,35}$`)

	// Parse the config
	config, err := d.initializeAzureConfig(configJSON, commonConfig)
	if err != nil {
		return fmt.Errorf("error initializing %s driver. %v", d.Name(), err)
	}
	d.Config = *config

	if err = d.populateConfigurationDefaults(&d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	if err = d.initializeStoragePools(); err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	if d.SDK, err = d.initializeAzureSDKClient(&d.Config); err != nil {
		return fmt.Errorf("error initializing %s SDK client. %v", d.Name(), err)
	}

	if err = d.validate(); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		i, err := strconv.ParseUint(d.Config.VolumeCreateTimeout, 10, 64)
		if err != nil {
			log.WithField("interval", d.Config.VolumeCreateTimeout).Errorf(
				"Invalid volume create timeout period. %v", err)
			return err
		}
		volumeCreateTimeout = time.Duration(i) * time.Second
	}
	d.volumeCreateTimeout = volumeCreateTimeout

	log.WithFields(log.Fields{
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
func (d *NFSStorageDriver) Terminate(string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	d.SDK.Terminate()

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NFSStorageDriver) populateConfigurationDefaults(config *drivers.AzureNFSStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "populateConfigurationDefaults", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> populateConfigurationDefaults")
		defer log.WithFields(fields).Debug("<<<< populateConfigurationDefaults")
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

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	if config.ExportRule == "" {
		config.ExportRule = defaultExportRule
	}
	log.WithFields(log.Fields{
		"StoragePrefix":   *config.StoragePrefix,
		"Size":            config.Size,
		"ServiceLevel":    config.ServiceLevel,
		"NfsMountOptions": config.NfsMountOptions,
		"LimitVolumeSize": config.LimitVolumeSize,
		"ExportRule":      config.ExportRule,
	}).Debugf("Configuration defaults")

	return nil
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NFSStorageDriver) initializeStoragePools() error {

	d.pools = make(map[string]*storage.Pool)

	if len(d.Config.Storage) == 0 {

		log.Debug("No vpools defined, reporting single pool.")

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
		pool.InternalAttributes[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes[Location] = d.Config.Location
		pool.InternalAttributes[VirtualNetwork] = d.Config.VirtualNetwork
		pool.InternalAttributes[Subnet] = d.Config.Subnet

		d.pools[pool.Name] = pool
	} else {

		log.Debug("One or more vpools defined.")

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

			serviceLevel := d.Config.ServiceLevel
			if vpool.ServiceLevel != "" {
				serviceLevel = vpool.ServiceLevel
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
			pool.InternalAttributes[ExportRule] = exportRule
			pool.InternalAttributes[Location] = location
			pool.InternalAttributes[VirtualNetwork] = vnet
			pool.InternalAttributes[Subnet] = subnet

			d.pools[pool.Name] = pool
		}
	}

	return nil
}

// initializeAzureConfig parses the Azure config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeAzureConfig(
	configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) (*drivers.AzureNFSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAzureConfig", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> initializeAzureConfig")
		defer log.WithFields(fields).Debug("<<<< initializeAzureConfig")
	}

	config := &drivers.AzureNFSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AzureNFSStorageDriverConfig object
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	return config, nil
}

// initializeAzureSDKClient returns an Azure SDK client.
func (d *NFSStorageDriver) initializeAzureSDKClient(
	config *drivers.AzureNFSStorageDriverConfig,
) (*sdk.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAzureSDKClient", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> initializeAzureSDKClient")
		defer log.WithFields(fields).Debug("<<<< initializeAzureSDKClient")
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
	if err := client.Init(d.pools); err != nil {
		return nil, err
	}

	return client, nil
}

// validate ensures the driver configuration and execution environment are valid and working
func (d *NFSStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	var err error

	// Ensure storage prefix is compatible with cloud service
	matched, err := regexp.MatchString(`[^a-zA-Z-]+`, *d.Config.StoragePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if matched {
		return fmt.Errorf("storage prefix may only contain letters and hyphens")
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level (it is allowed to be blank)
		switch pool.InternalAttributes[ServiceLevel] {
		case sdk.ServiceLevelStandard, sdk.ServiceLevelPremium, sdk.ServiceLevelUltra, "":
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

		// Validate default size
		if _, err = utils.ConvertSizeToBytes(pool.InternalAttributes[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		}
	}

	return nil
}

// Create a volume with the specified options
func (d *NFSStorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NFSStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
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
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == api.StateCreating {
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

	// TODO: remove this code once ANF can handle smaller volumes
	if sizeBytes < MinimumANFVolumeSizeBytes {

		log.WithFields(log.Fields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (100 GB).")

		sizeBytes = MinimumANFVolumeSizeBytes
		volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	serviceLevel := volConfig.ServiceLevel
	if serviceLevel == "" {
		serviceLevel = pool.InternalAttributes[ServiceLevel]
	}

	log.WithFields(log.Fields{
		"creationToken": name,
		"size":          sizeBytes,
		"serviceLevel":  serviceLevel,
	}).Debug("Creating volume.")

	apiExportRule := sdk.ExportRule{
		AllowedClients: pool.InternalAttributes[ExportRule],
		Cifs:           false,
		Nfsv3:          true,
		Nfsv4:          false,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := sdk.ExportPolicy{
		Rules: []sdk.ExportRule{apiExportRule},
	}

	createRequest := &sdk.FilesystemCreateRequest{
		Name:           volConfig.Name,
		Location:       pool.InternalAttributes[Location],
		VirtualNetwork: pool.InternalAttributes[VirtualNetwork],
		Subnet:         pool.InternalAttributes[Subnet],
		CreationToken:  name,
		ExportPolicy:   exportPolicy,
		Labels:         d.getTelemetryLabels(),
		ProtocolTypes:  []string{sdk.ProtocolTypeNFSv3},
		QuotaInBytes:   int64(sizeBytes),
		ServiceLevel:   serviceLevel,
		PoolID:         storagePool.Name,
	}

	// Create the volume
	volume, err := d.SDK.CreateVolume(createRequest)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(volume, name)
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NFSStorageDriver) CreateClone(volConfig *storage.VolumeConfig, _ *storage.Pool) error {

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
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
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
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get the source volume
	sourceVolume, err := d.SDK.GetVolumeByCreationToken(source)
	if err != nil {
		return fmt.Errorf("could not find source volume: %v", err)
	}

	var sourceSnapshot *sdk.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.SDK.GetSnapshotForVolume(sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.ProvisioningState != sdk.StateAvailable {
			return fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.ProvisioningState, sdk.StateAvailable)
		}

	} else {

		// No source snapshot specified, so create one
		snapName := time.Now().UTC().Format(storage.SnapshotNameFormat)
		snapshotCreateRequest := &sdk.SnapshotCreateRequest{
			FileSystemID: sourceVolume.FileSystemID,
			Volume:       sourceVolume,
			Name:         snapName,
			Location:     sourceVolume.Location,
		}

		sourceSnapshot, err = d.SDK.CreateSnapshot(snapshotCreateRequest)
		if err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		if err = d.SDK.WaitForSnapshotState(sourceSnapshot, sourceVolume, sdk.StateAvailable, []string{sdk.StateError},
			sdk.SnapshotTimeout); err != nil {
			return err
		}

		// Re-fetch the snapshot to populate the properties after create has completed
		sourceSnapshot, err = d.SDK.GetSnapshotForVolume(sourceVolume, snapName)
		if err != nil {
			return fmt.Errorf("could not retrieve newly-created snapshot")
		}
	}

	log.WithFields(log.Fields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshot.Name,
	}).Debug("Cloning volume.")

	cookie, err := d.SDK.GetCookieByCapacityPoolName(sourceVolume.CapacityPoolName)
	if err != nil {
		return fmt.Errorf("couldn't find cookie for volume %v", name)
	}

	createRequest := &sdk.FilesystemCreateRequest{
		Name:          volConfig.Name,
		Location:      sourceVolume.Location,
		CapacityPool:  sourceVolume.CapacityPoolName, // critical value for clone path
		CreationToken: name,
		ExportPolicy:  sourceVolume.ExportPolicy,
		Labels:        d.getTelemetryLabels(),
		ProtocolTypes: sourceVolume.ProtocolTypes,
		QuotaInBytes:  sourceVolume.QuotaInBytes,
		ServiceLevel:  sourceVolume.ServiceLevel,
		SnapshotID:    sourceSnapshot.SnapshotID,
		PoolID:        *cookie.StoragePoolName,
		Subnet:        sourceVolume.Subnet,
	}

	// Clone the volume
	clone, err := d.SDK.CreateVolume(createRequest)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(clone, name)
}

func (d *NFSStorageDriver) Import(volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "NFSStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> Import")
		defer log.WithFields(fields).Debug("<<<< Import")
	}

	// Get the volume
	creationToken := originalName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if _, err := d.SDK.RelabelVolume(volume, d.updateTelemetryLabels(volume)); err != nil {
			log.WithField("originalName", originalName).Errorf("Could not import volume, relabel failed: %v", err)
			return fmt.Errorf("could not import volume %s, relabel failed: %v", originalName, err)
		}
		_, err := d.SDK.WaitForVolumeState(volume, sdk.StateAvailable, []string{sdk.StateError}, d.defaultCreateTimeout())
		if err != nil {
			return fmt.Errorf("could not import volume %s: %v", originalName, err)
		}
	}

	// The ANF creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = creationToken

	return nil
}

func (d *NFSStorageDriver) Rename(name string, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "NFSStorageDriver",
			"name":    name,
			"newName": newName,
		}
		log.WithFields(fields).Debug(">>>> Rename")
		defer log.WithFields(fields).Debug("<<<< Rename")
	}

	// Rename is only needed for the import workflow, and we aren't currently renaming the
	// ANF volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) getTelemetryLabels() []string {

	telemetry := map[string]Telemetry{"trident": *d.getTelemetry()}
	telemetryLabel := ""
	telemetryJSON, err := json.Marshal(telemetry)
	if err != nil {
		log.Errorf("Failed to marshal telemetry: %+v", telemetryLabel)
	} else {
		telemetryLabel = strings.Replace(string(telemetryJSON), " ", "", -1)
	}
	return []string{telemetryLabel}
}

func (d *NFSStorageDriver) isTelemetryLabel(label string) bool {

	var telemetry map[string]Telemetry
	if err := json.Unmarshal([]byte(label), &telemetry); err != nil {
		return false
	}
	if _, ok := telemetry["trident"]; !ok {
		return false
	}
	return true
}

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) updateTelemetryLabels(volume *sdk.FileSystem) []string {

	newLabels := d.getTelemetryLabels()

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
func (d *NFSStorageDriver) waitForVolumeCreate(volume *sdk.FileSystem, volumeName string) error {

	state, err := d.SDK.WaitForVolumeState(volume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		// Don't leave a CVS volume laying around in error state
		if state == api.StateCreating {
			log.WithFields(log.Fields{
				"volume": volumeName,
			}).Debug("Volume is in creating state.")
			return utils.VolumeCreatingError(err.Error())
		}

		// Don't leave a CVS volume in a non-transitional state laying around in error state
		if !api.IsTransitionalState(state) {
			errDelete := d.SDK.DeleteVolume(volume)
			if errDelete != nil {
				log.WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDelete)
			}
		} else {
			// Wait for deletion to complete
			_, errDeleteWait := d.SDK.WaitForVolumeState(
				volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
			if errDeleteWait != nil {
				log.WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDeleteWait)
			}
		}
	}

	return nil
}

// Destroy deletes a volume.
func (d *NFSStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NFSStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	// If volume doesn't exist, return success
	// 'name' is in fact 'creationToken' here.
	volumeExists, extantVolume, err := d.SDK.VolumeExistsByCreationToken(name)

	if err != nil {
		return err
	}
	if !volumeExists {
		log.WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.ProvisioningState == api.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.SDK.WaitForVolumeState(extantVolume, api.StateDeleted, []string{api.StateError}, d.volumeCreateTimeout)
		return err
	}

	// Delete the volume
	if err = d.SDK.DeleteVolume(extantVolume); err != nil {
		return err
	}

	// Wait for deletion to complete
	_, err = d.SDK.WaitForVolumeState(extantVolume, sdk.StateDeleted, []string{sdk.StateError}, d.defaultTimeout())
	return err
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NFSStorageDriver) Publish(name string, publishInfo *utils.VolumePublishInfo) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "NFSStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	// Get the volume
	creationToken := name

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	mountTargets, err := d.SDK.GetMountTargetsForVolume(volume)
	if err != nil {
		return fmt.Errorf("could not read mount targets for volume %s: %v", name, err)
	}
	if len(*mountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = "/" + volume.CreationToken
	publishInfo.NfsServerIP = (*mountTargets)[0].IPAddress
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = d.Config.NfsMountOptions

	return nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NFSStorageDriver) GetSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshot")
		defer log.WithFields(fields).Debug("<<<< GetSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.SDK.GetSnapshotsForVolume(volume)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == internalSnapName {

			created := snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat)

			log.WithFields(log.Fields{
				"snapshotName": internalSnapName,
				"volumeName":   internalVolName,
				"created":      created,
			}).Debug("Found snapshot.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   created,
				SizeBytes: volume.QuotaInBytes,
			}, nil
		}
	}

	log.WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Warning("Snapshot not found.")

	return nil, nil
}

// Return the list of snapshots associated with the specified volume
func (d *NFSStorageDriver) GetSnapshots(volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {

	internalVolName := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NFSStorageDriver",
			"volumeName": internalVolName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshots")
		defer log.WithFields(fields).Debug("<<<< GetSnapshots")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.SDK.GetSnapshotsForVolume(volume)
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
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NFSStorageDriver) CreateSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		log.WithFields(fields).Debug(">>>> CreateSnapshot")
		defer log.WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	// Check if volume exists
	volumeExists, sourceVolume, err := d.SDK.VolumeExistsByCreationToken(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
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

	snapshot, err := d.SDK.CreateSnapshot(snapshotCreateRequest)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Wait for snapshot creation to complete
	if err = d.SDK.WaitForSnapshotState(snapshot, sourceVolume, sdk.StateAvailable, []string{sdk.StateError}, sdk.SnapshotTimeout); err != nil {
		return nil, err
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: sourceVolume.QuotaInBytes,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NFSStorageDriver) RestoreSnapshot(snapConfig *storage.SnapshotConfig) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		log.WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer log.WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.SDK.GetSnapshotForVolume(volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	return d.SDK.RestoreSnapshot(volume, snapshot)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NFSStorageDriver) DeleteSnapshot(snapConfig *storage.SnapshotConfig) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		log.WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer log.WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.SDK.GetSnapshotForVolume(volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	if err = d.SDK.DeleteSnapshot(volume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	if err := d.SDK.WaitForSnapshotState(snapshot, volume, sdk.StateDeleted, []string{sdk.StateError},
		sdk.SnapshotTimeout); err != nil {
		return err
	}

	return nil
}

// Return the list of volumes associated with this tenant
func (d *NFSStorageDriver) List() ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> List")
		defer log.WithFields(fields).Debug("<<<< List")
	}

	volumes, err := d.SDK.GetVolumes()
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

// Test for the existence of a volume
func (d *NFSStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.SDK.GetVolumeByCreationToken(name)

	return err
}

// Resize increases a volume's quota
func (d *NFSStorageDriver) Resize(volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NFSStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		log.WithFields(fields).Debug(">>>> Resize")
		defer log.WithFields(fields).Debug("<<<< Resize")
	}

	// Get the volume
	creationToken := name

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
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
	if _, _, err = drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Resize the volume
	_, err = d.SDK.ResizeVolume(volume, int64(sizeBytes))
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

// Retrieve storage capabilities and register pools with specified backend.
func (d *NFSStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {

	backend.Name = d.backendName()

	for _, pool := range d.pools {
		pool.Backend = backend
		backend.AddStoragePool(pool)
	}

	return nil
}

func (d *NFSStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
}

// Retrieve storage backend physical pools
func (d *NFSStorageDriver) GetStorageBackendPhysicalPoolNames() []string {
	return []string{}
}

func (d *NFSStorageDriver) GetInternalVolumeName(name string) string {

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

func (d *NFSStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "CreateFollowup",
			"Type":   "NFSStorageDriver",
			"name":   volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> CreateFollowup")
		defer log.WithFields(fields).Debug("<<<< CreateFollowup")
	}

	// Get the volume
	creationToken := volConfig.InternalName

	volume, err := d.SDK.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Ensure volume is in a good state
	if volume.ProvisioningState == sdk.StateError {
		return fmt.Errorf("volume %s is in %s state: %s", creationToken, sdk.StateError, volume.ProvisioningState)
	}

	mountTargets, err := d.SDK.GetMountTargetsForVolume(volume)
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

func (d *NFSStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NFSStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AzureConfig = &d.Config
}

func (d *NFSStorageDriver) GetExternalConfig() interface{} {

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AzureNFSStorageDriverConfig
	drivers.Clone(d.Config, &cloneConfig)
	cloneConfig.ClientSecret = "" // redact the Secret
	return cloneConfig
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NFSStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.SDK.GetVolumeByName(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NFSStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "GetVolumeExternalWrappers", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> GetVolumeExternalWrappers")
		defer log.WithFields(fields).Debug("<<<< GetVolumeExternalWrappers")
	}

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.SDK.GetVolumes()
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
func (d *NFSStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	_, ok := driverOrig.(*NFSStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	return bitmap
}

func (d *NFSStorageDriver) ReconcileNodeAccess(nodes []*utils.Node, backendUUID string) error {

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
		log.WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer log.WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	return nil
}
