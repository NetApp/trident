// Copyright 2019 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/mitchellh/copystructure"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes    = 1000000000    // 1 GB
	MinimumCVSVolumeSizeBytes = 1099511627776 // 1 TiB
	MinimumAPIVersion         = "1.1.5"
	MinimumSDEVersion         = "1.184.14"

	defaultServiceLevel    = api.UserServiceLevel1
	defaultNfsMountOptions = "-o nfsvers=3"
	defaultSecurityStyle   = "unix"
	defaultSnapshotDir     = "false"
	defaultSnapshotReserve = ""
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
)

// NFSStorageDriver is for storage provisioning using Cloud Volumes Service in GCP
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.GCPNFSStorageDriverConfig
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

func (d *NFSStorageDriver) GetConfig() *drivers.GCPNFSStorageDriverConfig {
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
	return drivers.GCPNFSStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance
func (d *NFSStorageDriver) defaultBackendName() string {
	return fmt.Sprintf("%s_%s",
		strings.Replace(d.Name(), "-", "", -1),
		d.Config.APIKey.PrivateKeyID[0:5])
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

	return fmt.Sprintf("%s_%s",
		d.backendName(),
		strings.Replace(name, "-", "", -1))
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
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	d.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// Parse the config
	config, err := d.initializeGCPConfig(configJSON, commonConfig)
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

	if d.API, err = d.initializeGCPAPIClient(&d.Config); err != nil {
		return fmt.Errorf("error initializing %s API client. %v", d.Name(), err)
	}

	if err = d.validate(); err != nil {
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
func (d *NFSStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NFSStorageDriver) populateConfigurationDefaults(config *drivers.GCPNFSStorageDriverConfig) error {

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

	if config.Network == "" {
		config.Network = defaultNetwork
	}

	// VolumeCreateTimeoutSeconds is the timeout value in seconds.
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
		pool.InternalAttributes[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes[Network] = d.Config.Network
		pool.InternalAttributes[Region] = d.Config.Region
		pool.InternalAttributes[Zone] = d.Config.Zone

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

			size := d.Config.Size
			if vpool.Size != "" {
				size = vpool.Size
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
			pool.InternalAttributes[Network] = network
			pool.InternalAttributes[Region] = region
			pool.InternalAttributes[Zone] = zone

			d.pools[pool.Name] = pool
		}
	}

	return nil
}

// initializeGCPConfig parses the GCP config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeGCPConfig(
	configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) (*drivers.GCPNFSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeGCPConfig", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> initializeGCPConfig")
		defer log.WithFields(fields).Debug("<<<< initializeGCPConfig")
	}

	config := &drivers.GCPNFSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into GCPNFSStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	return config, nil
}

// initializeGCPAPIClient returns an GCP API client.
func (d *NFSStorageDriver) initializeGCPAPIClient(
	config *drivers.GCPNFSStorageDriverConfig,
) (*api.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeGCPAPIClient", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> initializeGCPAPIClient")
		defer log.WithFields(fields).Debug("<<<< initializeGCPAPIClient")
	}

	client := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

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

	// Ensure API region is set
	if d.Config.APIRegion == "" {
		return errors.New("apiRegion in config must be specified")
	}

	// Validate API version
	if d.apiVersion, d.sdeVersion, err = d.API.GetVersion(); err != nil {
		return err
	}

	if _, err := d.API.GetVolumes(); err != nil {
		return fmt.Errorf("could not read volumes in %s region; %v", d.Config.APIRegion, err)
	}

	if d.apiVersion.LessThan(utils.MustParseSemantic(MinimumAPIVersion)) {
		return fmt.Errorf("API version is %s, at least %s is required", d.apiVersion.String(), MinimumAPIVersion)
	}
	if d.sdeVersion.LessThan(utils.MustParseSemantic(MinimumSDEVersion)) {
		return fmt.Errorf("SDE version is %s, at least %s is required", d.sdeVersion.String(), MinimumSDEVersion)
	}

	log.WithField("region", d.Config.APIRegion).Debug("REST API access OK.")

	// Validate driver-level attributes

	// Ensure storage prefix is compatible with cloud service
	matched, err := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9-]{0,70}$`, *d.Config.StoragePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		return fmt.Errorf("storage prefix may only contain letters/digits/hyphens and must begin with a letter")
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level
		if !api.IsValidUserServiceLevel(pool.InternalAttributes[ServiceLevel]) {
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
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(name)
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

		log.WithFields(log.Fields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (1 TiB).")

		sizeBytes = MinimumCVSVolumeSizeBytes
		volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	userServiceLevel := volConfig.ServiceLevel
	if userServiceLevel == "" {
		userServiceLevel = pool.InternalAttributes[ServiceLevel]
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
		volConfig.SnapshotReserve = snapshotReserve
	}

	// Take network from volume config first (handles Docker case), then from pool
	network := volConfig.Network
	if network == "" {
		network = pool.InternalAttributes[Network]
		volConfig.Network = network
	}
	gcpNetwork := fmt.Sprintf("projects/%s/global/networks/%s", d.Config.ProjectNumber, network)

	log.WithFields(log.Fields{
		"creationToken":   name,
		"size":            sizeBytes,
		"network":         network,
		"serviceLevel":    userServiceLevel,
		"snapshotDir":     snapshotDirBool,
		"snapshotReserve": snapshotReserve,
	}).Debug("Creating volume.")

	apiExportRule := api.ExportRule{
		AllowedClients: pool.InternalAttributes[ExportRule],
		Access:         api.AccessReadWrite,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
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
		Labels:            d.getTelemetryLabels(),
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      int64(sizeBytes),
		SecurityStyle:     defaultSecurityStyle,
		ServiceLevel:      apiServiceLevel,
		SnapshotDirectory: snapshotDirBool,
		SnapshotPolicy:    snapshotPolicy,
		SnapReserve:       snapshotReservePtr,
		Network:           gcpNetwork,
	}

	// Create the volume
	if err := d.API.CreateVolume(createRequest); err != nil {
		return err
	}

	// Get the volume
	newVolume, err := d.API.GetVolumeByCreationToken(name)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(newVolume, name)
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
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(name)
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
	sourceVolume, err := d.API.GetVolumeByCreationToken(source)
	if err != nil {
		return fmt.Errorf("could not find source volume: %v", err)
	}
	if sourceVolume.Region != d.Config.APIRegion {
		return fmt.Errorf("volume %s requested in region %s, but it already exists in region %s",
			name, d.Config.APIRegion, sourceVolume.Region)
	}

	// Get the source volume by ID to get all details
	sourceVolume, err = d.API.GetVolumeByID(sourceVolume.VolumeID)
	if err != nil {
		return fmt.Errorf("could not get source volume details: %v", err)
	}

	var sourceSnapshot *api.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.API.GetSnapshotForVolume(sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.LifeCycleState != api.StateAvailable {
			return fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.LifeCycleState, api.StateAvailable)
		}

	} else {

		newSnapName := time.Now().UTC().Format(storage.SnapshotNameFormat)

		// No source snapshot specified, so create one
		snapshotCreateRequest := &api.SnapshotCreateRequest{
			VolumeID: sourceVolume.VolumeID,
			Name:     newSnapName,
		}

		if err = d.API.CreateSnapshot(snapshotCreateRequest); err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		sourceSnapshot, err = d.API.GetSnapshotForVolume(sourceVolume, newSnapName)
		if err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.API.WaitForSnapshotState(sourceSnapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return err
		}
	}

	network := volConfig.Network
	if network == "" {
		log.Debugf("Network not found in volume config, using '%s'.", d.Config.Network)
		network = d.Config.Network
	}
	gcpNetwork := fmt.Sprintf("projects/%s/global/networks/%s", d.Config.ProjectNumber, network)

	log.WithFields(log.Fields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshot.Name,
	}).Debug("Cloning volume.")

	createRequest := &api.VolumeCreateRequest{
		Name:              volConfig.Name,
		Region:            sourceVolume.Region,
		CreationToken:     name,
		ExportPolicy:      sourceVolume.ExportPolicy,
		Labels:            d.getTelemetryLabels(),
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		QuotaInBytes:      sourceVolume.QuotaInBytes,
		SecurityStyle:     defaultSecurityStyle,
		ServiceLevel:      sourceVolume.ServiceLevel,
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SnapshotPolicy:    sourceVolume.SnapshotPolicy,
		SnapReserve:       &sourceVolume.SnapReserve,
		SnapshotID:        sourceSnapshot.SnapshotID,
		Network:           gcpNetwork,
	}

	// Clone the volume
	if err := d.API.CreateVolume(createRequest); err != nil {
		return err
	}

	// Get the volume
	clone, err := d.API.GetVolumeByCreationToken(name)
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if _, err := d.API.RelabelVolume(volume, d.updateTelemetryLabels(volume)); err != nil {
			log.WithField("originalName", originalName).Errorf("Could not import volume, relabel failed: %v", err)
			return fmt.Errorf("could not import volume %s, relabel failed: %v", originalName, err)
		}
		_, err := d.API.WaitForVolumeState(volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return fmt.Errorf("could not import volume %s: %v", originalName, err)
		}
	}

	// The CVS creation token cannot be changed, so use it as the internal name
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
	// CVS volume when importing, so do nothing here lest we set the volume name incorrectly
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
	err := json.Unmarshal([]byte(label), &telemetry)
	if err != nil {
		return false
	}
	if _, ok := telemetry["trident"]; !ok {
		return false
	}
	return true
}

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) updateTelemetryLabels(volume *api.Volume) []string {

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
// This method is used by both Create & CreateClone.
func (d *NFSStorageDriver) waitForVolumeCreate(volume *api.Volume, volumeName string) error {

	state, err := d.API.WaitForVolumeState(volume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		if state == api.StateCreating {
			log.WithFields(log.Fields{
				"volume": volumeName,
			}).Debug("Volume is in creating state.")
			return utils.VolumeCreatingError(err.Error())
		}

		// Don't leave a CVS volume in a non-transitional state laying around in error state
		if !api.IsTransitionalState(state) {
			errDelete := d.API.DeleteVolume(volume)
			if errDelete != nil {
				log.WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDelete)
			} else {
				// Wait for deletion to complete
				_, errDeleteWait := d.API.WaitForVolumeState(
					volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
				if errDeleteWait != nil {
					log.WithFields(log.Fields{
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
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(name)
	if err != nil {
		return err
	}
	if !volumeExists {
		log.WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.LifeCycleState == api.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.API.WaitForVolumeState(extantVolume, api.StateDeleted, []string{api.StateError}, d.volumeCreateTimeout)
		return err
	}

	// Get the volume
	volume, err := d.API.GetVolumeByCreationToken(name)
	if err != nil {
		return err
	}

	// Delete the volume
	if err = d.API.DeleteVolume(volume); err != nil {
		return err
	}

	// Wait for deletion to complete
	_, err = d.API.WaitForVolumeState(volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	if len(volume.MountPoints) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Add fields needed by Attach
	publishInfo.NfsServerIP = (volume.MountPoints)[0].Server
	publishInfo.NfsPath = (volume.MountPoints)[0].Export
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = d.Config.NfsMountOptions

	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.API.GetSnapshotsForVolume(volume)
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.API.GetSnapshotsForVolume(volume)
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
	volumeExists, sourceVolume, err := d.API.VolumeExistsByCreationToken(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Get the source volume by ID to get all details
	sourceVolume, err = d.API.GetVolumeByID(sourceVolume.VolumeID)
	if err != nil {
		return nil, fmt.Errorf("could not get source volume details: %v", err)
	}

	// Construct a snapshot request
	snapshotCreateRequest := &api.SnapshotCreateRequest{
		VolumeID: sourceVolume.VolumeID,
		Name:     internalSnapName,
	}

	if err := d.API.CreateSnapshot(snapshotCreateRequest); err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	snapshot, err := d.API.GetSnapshotForVolume(sourceVolume, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not find snapshot: %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.API.WaitForSnapshotState(snapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return nil, err
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.Format(storage.SnapshotTimestampFormat),
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.API.GetSnapshotForVolume(volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	return d.API.RestoreSnapshot(volume, snapshot)
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.API.GetSnapshotForVolume(volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	if err := d.API.DeleteSnapshot(volume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	return d.API.WaitForSnapshotState(snapshot, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
}

// Return the list of volumes associated with this tenant
func (d *NFSStorageDriver) List() ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> List")
		defer log.WithFields(fields).Debug("<<<< List")
	}

	volumes, err := d.API.GetVolumes()
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
func (d *NFSStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NFSStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.API.GetVolumeByCreationToken(name)

	return err
}

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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// If the volume state isn't Available, return an error
	if volume.LifeCycleState != api.StateAvailable {
		return fmt.Errorf("volume %s state is %s, not available", creationToken, volume.LifeCycleState)
	}

	// If the volume is already the requested size, there's nothing to do
	if int64(sizeBytes) == volume.QuotaInBytes {
		return nil
	}

	// Make sure we're not shrinking the volume
	if int64(sizeBytes) < volume.QuotaInBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volume.QuotaInBytes)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	if _, _, err := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Resize the volume
	if _, err := d.API.ResizeVolume(volume, int64(sizeBytes)); err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
	}

	// Wait for resize operation to complete
	_, err = d.API.WaitForVolumeState(volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return err
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
	} else if d.csiRegexp.MatchString(name) {
		// If the name is from CSI (i.e. contains a UUID), just use it as-is
		log.WithField("volumeInternal", name).Debug("Using volume name as internal name.")
		return name
	} else {
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, ".", "-", -1) // CVS disallows periods
		log.WithField("volumeInternal", internal).Debug("Modified volume name for internal name.")
		return internal
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

	volume, err := d.API.GetVolumeByCreationToken(creationToken)
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

func (d *NFSStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NFSStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.GCPConfig = &d.Config
}

func (d *NFSStorageDriver) GetExternalConfig() interface{} {

	// Clone the config so we don't risk altering the original

	clone, err := copystructure.Copy(d.Config)
	if err != nil {
		log.Errorf("Could not clone GCP backend config; %v", err)
		return drivers.GCPNFSStorageDriverConfig{}
	}

	cloneConfig, ok := clone.(drivers.GCPNFSStorageDriverConfig)
	if !ok {
		log.Errorf("Could not cast GCP backend config; %v", err)
		return drivers.GCPNFSStorageDriverConfig{}
	}

	cloneConfig.APIKey = drivers.GCPPrivateKey{} // redact the API key
	return cloneConfig
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NFSStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	creationToken := name
	volumeAttrs, err := d.API.GetVolumeByCreationToken(creationToken)
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

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.API.GetVolumes()
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
		SnapshotDir:     "true",
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
func (d *NFSStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	_, ok := driverOrig.(*NFSStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	return bitmap
}
