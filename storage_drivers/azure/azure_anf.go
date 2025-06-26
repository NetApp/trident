// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nfs"
)

const (
	MinimumANFVolumeSizeBytes = capacity.OneGiB * 50 // 50 GiB

	defaultUnixPermissions         = "" // TODO (cknight): change to "0777" when whitelisted permissions feature reaches GA
	defaultNfsMountOptions         = "nfsvers=3"
	defaultKerberosNfsMountOptions = "nfsvers=4.1"
	defaultSnapshotDir             = ""
	defaultLimitVolumeSize         = ""
	defaultExportRule              = "0.0.0.0/0"
	defaultVolumeSizeStr           = "53687091200" // 50 GiB
	defaultNetworkFeatures         = ""            // Leave empty, some regions may never support this
	defaultMaxThroughput           = "4"           // MiB/s
	defaultQOSType                 = api.QOSAuto

	// Constants for internal pool attributes

	Size            = "size"
	UnixPermissions = "unixPermissions"
	ServiceLevel    = "serviceLevel"
	SnapshotDir     = "snapshotDir"
	ExportRule      = "exportRule"
	VirtualNetwork  = "virtualNetwork"
	NetworkFeatures = "networkFeatures"
	Subnet          = "subnet"
	ResourceGroups  = "resourceGroups"
	NetappAccounts  = "netappAccounts"
	CapacityPools   = "capacityPools"
	MaxThroughput   = "maxThroughput"
	Kerberos        = "kerberos"
	QOSType         = "qosType"

	nfsVersion3  = "3"
	nfsVersion4  = "4"
	nfsVersion41 = "4.1"
)

var (
	supportedNFSVersions = []string{nfsVersion3, nfsVersion4, nfsVersion41}

	storagePrefixRegex       = regexp.MustCompile(`^$|^[a-zA-Z][a-zA-Z-]*$`)
	volumeNameRegex          = regexp.MustCompile(`^[a-zA-Z][a-zA-Z\d-_]{0,63}$`)
	volumeCreationTokenRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z\d-]{0,79}$`)
	csiRegex                 = regexp.MustCompile(`^pvc-[\da-fA-F]{8}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{12}$`)
)

// NASStorageDriver is for storage provisioning using the Azure NetApp Files service.
type NASStorageDriver struct {
	initialized         bool
	Config              drivers.AzureNASStorageDriverConfig
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
func (d *NASStorageDriver) Name() string {
	return tridentconfig.AzureNASStorageDriverName
}

// GetConfig returns the config of this driver.
func (d *NASStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

// defaultBackendName returns the default name of the backend managed by this driver instance.
func (d *NASStorageDriver) defaultBackendName() string {
	id := crypto.RandomString(6)
	if len(d.Config.ClientID) > 5 {
		id = d.Config.ClientID[0:5]
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

// validateVolumeName checks that the name of a new volume matches the requirements of an ANF volume name.
func (d *NASStorageDriver) validateVolumeName(name string) error {
	if !volumeNameRegex.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be 1-64 characters long, "+
			"begin with a letter, and contain only letters, digits, hyphens, and underscores", name)
	}
	return nil
}

// validateCreationToken checks that the creation token of a new volume matches the requirements of a creation token.
func (d *NASStorageDriver) validateCreationToken(name string) error {
	if !volumeCreationTokenRegex.MatchString(name) {
		return fmt.Errorf("volume internal name '%s' is not allowed; it must be 1-80 characters long, "+
			"begin with a letter, and contain only letters, digits, and hyphens", name)
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
		return api.VolumeCreateTimeout
	}
}

// defaultTimeout controls the driver timeout for most workflows.
func (d *NASStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return api.DefaultTimeout
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

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %v", err)
	}
	d.Config.BackendPools = pools

	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		i, parseErr := convert.ToPositiveInt64(d.Config.VolumeCreateTimeout)
		if parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.VolumeCreateTimeout).WithError(parseErr).Error(
				"Invalid volume create timeout period.")
			return parseErr
		} else {
			volumeCreateTimeout = time.Duration(i) * time.Second
		}
	}
	d.volumeCreateTimeout = volumeCreateTimeout

	Logc(ctx).WithFields(LogFields{
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

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config.
func (d *NASStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AzureNASStorageDriverConfig,
) {
	fields := LogFields{"Method": "populateConfigurationDefaults", "Type": "NASStorageDriver"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> populateConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< populateConfigurationDefaults")

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
		if config.Kerberos != "" {
			config.NfsMountOptions = defaultKerberosNfsMountOptions
		} else {
			config.NfsMountOptions = defaultNfsMountOptions
		}
	}

	if config.SnapshotDir != "" {
		// Set the snapshotDir provided in the config
		snapDirFormatted, err := convert.ToFormattedBool(config.SnapshotDir)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir: %v.", config.SnapshotDir)
		}
		config.SnapshotDir = snapDirFormatted
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	if config.ExportRule == "" {
		config.ExportRule = defaultExportRule
	}

	if config.NetworkFeatures == "" {
		config.NetworkFeatures = defaultNetworkFeatures
	}

	if config.NASType == "" {
		config.NASType = sa.NFS
	}

	if config.QOSType == "" {
		config.QOSType = defaultQOSType
	}

	if config.MaxThroughput == "" {
		config.MaxThroughput = defaultMaxThroughput
	}

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":   *config.StoragePrefix,
		"Size":            config.Size,
		"UnixPermissions": config.UnixPermissions,
		"ServiceLevel":    config.ServiceLevel,
		"NfsMountOptions": config.NfsMountOptions,
		"SnapshotDir":     config.SnapshotDir,
		"LimitVolumeSize": config.LimitVolumeSize,
		"ExportRule":      config.ExportRule,
		"QOSType":         config.QOSType,
		"MaxThroughput":   config.MaxThroughput,
	}).Debugf("Configuration defaults")

	return
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NASStorageDriver) initializeStoragePools(ctx context.Context) {
	d.pools = make(map[string]storage.Pool)

	// If snapshotDir is provided, ensure it is lower case
	if d.Config.SnapshotDir != "" {
		snapDirFormatted, err := convert.ToFormattedBool(d.Config.SnapshotDir)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir: %v.", d.Config.SnapshotDir)
		}
		d.Config.SnapshotDir = snapDirFormatted
	}

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
		pool.InternalAttributes()[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes()[VirtualNetwork] = d.Config.VirtualNetwork
		pool.InternalAttributes()[NetworkFeatures] = d.Config.NetworkFeatures
		pool.InternalAttributes()[Subnet] = d.Config.Subnet
		pool.InternalAttributes()[ResourceGroups] = strings.Join(d.Config.ResourceGroups, ",")
		pool.InternalAttributes()[NetappAccounts] = strings.Join(d.Config.NetappAccounts, ",")
		pool.InternalAttributes()[CapacityPools] = strings.Join(d.Config.CapacityPools, ",")
		pool.InternalAttributes()[Kerberos] = d.Config.Kerberos
		pool.InternalAttributes()[QOSType] = d.Config.QOSType
		pool.InternalAttributes()[MaxThroughput] = d.Config.MaxThroughput

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
				snapDirFormatted, err := convert.ToFormattedBool(vpool.SnapshotDir)
				if err != nil {
					Logc(ctx).WithError(err).Errorf("Invalid boolean value for vpool's snapshotDir: %v.",
						vpool.SnapshotDir)
				}
				snapshotDir = snapDirFormatted
			}

			exportRule := d.Config.ExportRule
			if vpool.ExportRule != "" {
				exportRule = vpool.ExportRule
			}

			vnet := d.Config.VirtualNetwork
			if vpool.VirtualNetwork != "" {
				vnet = vpool.VirtualNetwork
			}

			networkFeatures := d.Config.NetworkFeatures
			if vpool.NetworkFeatures != "" {
				networkFeatures = vpool.NetworkFeatures
			}

			subnet := d.Config.Subnet
			if vpool.Subnet != "" {
				subnet = vpool.Subnet
			}

			kerberos := d.Config.Kerberos
			if vpool.Kerberos != "" {
				kerberos = vpool.Kerberos
			}

			qosType := d.Config.QOSType
			if vpool.QOSType != "" {
				qosType = vpool.QOSType
			}

			maxThroughput := d.Config.MaxThroughput
			if vpool.MaxThroughput != "" {
				maxThroughput = vpool.MaxThroughput
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)

			nasType := d.Config.NASType
			if vpool.NASType != "" {
				nasType = vpool.NASType
			}

			pool.Attributes()[sa.NASType] = sa.NewStringOffer(nasType)

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
			pool.InternalAttributes()[ExportRule] = exportRule
			pool.InternalAttributes()[VirtualNetwork] = vnet
			pool.InternalAttributes()[NetworkFeatures] = networkFeatures
			pool.InternalAttributes()[Subnet] = subnet
			pool.InternalAttributes()[ResourceGroups] = strings.Join(resourceGroups, ",")
			pool.InternalAttributes()[NetappAccounts] = strings.Join(netappAccounts, ",")
			pool.InternalAttributes()[CapacityPools] = strings.Join(capacityPools, ",")
			pool.InternalAttributes()[Kerberos] = kerberos
			pool.InternalAttributes()[QOSType] = qosType
			pool.InternalAttributes()[MaxThroughput] = maxThroughput

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

// initializeAzureConfig parses the Azure config, mixing in the specified common config.
func (d *NASStorageDriver) initializeAzureConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.AzureNASStorageDriverConfig, error) {
	fields := LogFields{"Method": "initializeAzureConfig", "Type": "NASStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeAzureConfig")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeAzureConfig")

	config := &drivers.AzureNASStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AzureNASStorageDriverConfig object
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
func (d *NASStorageDriver) initializeAzureSDKClient(
	ctx context.Context, config *drivers.AzureNASStorageDriverConfig,
) error {
	fields := LogFields{"Method": "initializeAzureSDKClient", "Type": "NASStorageDriver"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeAzureSDKClient")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeAzureSDKClient")

	sdkTimeout := api.DefaultSDKTimeout
	if config.SDKTimeout != "" {
		i, parseErr := convert.ToPositiveInt64(d.Config.SDKTimeout)
		if parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.SDKTimeout).WithError(parseErr).Error(
				"Invalid value for SDK timeout.")
			return parseErr
		} else {
			sdkTimeout = time.Duration(i) * time.Second
		}
	}

	maxCacheAge := api.DefaultMaxCacheAge
	if config.MaxCacheAge != "" {
		i, parseErr := convert.ToPositiveInt64(d.Config.MaxCacheAge)
		if parseErr != nil {
			Logc(ctx).WithField("interval", d.Config.MaxCacheAge).WithError(parseErr).Error(
				"Invalid value for max cache age.")
			return parseErr
		} else {
			maxCacheAge = time.Duration(i) * time.Second
		}
	}

	clientConfig := api.ClientConfig{
		SubscriptionID: config.SubscriptionID,
		AzureAuthConfig: azclient.AzureAuthConfig{
			AADClientID:     config.ClientID,
			AADClientSecret: config.ClientSecret,
		},
		TenantID:          config.TenantID,
		Location:          config.Location,
		StorageDriverName: config.StorageDriverName,
		DebugTraceFlags:   config.DebugTraceFlags,
		SDKTimeout:        sdkTimeout,
		MaxCacheAge:       maxCacheAge,
	}

	// Try ANF driver initialization with Azure workload identity followed by Azure managed identity,
	// when both are not present, default to credentials present in the backend configuration file.

	// Azure workload identity
	// If cloud identity is provided and cloud provider is set to 'Azure' during the installation,
	// we can use AZURE_CLIENT_ID,AZURE_TENANT_ID,AZURE_FEDERATED_TOKEN_FILE and AZURE_AUTHORITY_HOST
	// environment variables injected by workload identity webhook for initialization of ANF driver.

	if os.Getenv("AZURE_CLIENT_ID") != "" && os.Getenv("AZURE_TENANT_ID") != "" &&
		os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" && os.Getenv("AZURE_AUTHORITY_HOST") != "" {
		Logc(ctx).Info("Using Azure workload identity.")
	} else {
		// Azure managed identity
		// If cloud provider is set to 'Azure' and cloud identity is not provided during the installation,
		// we read the contents of AZURE_CREDENTIAL_FILE to initialize the ANF driver.
		if config.ClientSecret == "" && config.ClientID == "" && os.Getenv("AZURE_CREDENTIAL_FILE") != "" {
			credFilePath := os.Getenv("AZURE_CREDENTIAL_FILE")
			Logc(ctx).WithField("credFilePath", credFilePath).Info("Using Azure credential config file.")

			credFile, err := os.ReadFile(credFilePath)
			if err != nil {
				return fmt.Errorf("error reading from azure config file: %w", err)
			}
			if err = json.Unmarshal(credFile, &clientConfig); err != nil {
				return fmt.Errorf("error parsing azureAuthConfig: %w", err)
			}

			// Set SubscriptionID and Location.
			d.Config.SubscriptionID = clientConfig.SubscriptionID
			d.Config.Location = clientConfig.Location
		}
	}

	client, err := api.NewDriver(clientConfig)
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
func (d *NASStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Ensure that networkFeatures is not set to Basic when customer encryption keys are passed
	if err := validateNetworkFeatures(d.Config.NetworkFeatures, d.Config.CustomerEncryptionKeys); err != nil {
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
				return fmt.Errorf("invalid boolean value for snapshotDir in pool %s; %v", poolName, err)
			}
		}

		// Validate unix permissions
		if pool.InternalAttributes()[UnixPermissions] != "" {
			err := filesystem.ValidateOctalUnixPermissions(pool.InternalAttributes()[UnixPermissions])
			if err != nil {
				return fmt.Errorf("invalid value for unixPermissions in pool %s; %v", poolName, err)
			}
		}

		// Validate default size
		if _, err := capacity.ToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s; %v", poolName, err)
		}

		// Validate pool labels
		if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength); err != nil {
			return fmt.Errorf("invalid value for label in pool %s; %v", poolName, err)
		}

		// Validate vnet features
		switch pool.InternalAttributes()[NetworkFeatures] {
		case "", api.NetworkFeaturesBasic, api.NetworkFeaturesStandard:
			break
		default:
			return fmt.Errorf("invalid value for networkFeatures in pool %s", poolName)
		}

		// Validate Kerberos
		if pool.InternalAttributes()[Kerberos] != "" {
			if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption); err != nil {
				// Log a warning to avoid putting the backend into a failed state.
				Logc(ctx).WithFields(LogFields{
					"attribute": Kerberos,
					"value":     pool.InternalAttributes()[Kerberos],
				}).WithError(err).Warning("Pool attribute requires ACP; workflows using this option may fail.")
			}
		}

		// Validate QOSType
		switch pool.InternalAttributes()[QOSType] {
		case api.QOSAuto, api.QOSManual:
			break
		default:
			return fmt.Errorf("invalid value for qosType in pool %s", poolName)
		}

		// Validate max throughput
		if pool.InternalAttributes()[MaxThroughput] != "" {
			if _, err := strconv.Atoi(pool.InternalAttributes()[MaxThroughput]); err != nil {
				return fmt.Errorf("invalid value for maxThroughput in pool %s", poolName)
			}
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

	// Check if this volume landed on a Kerberos-enabled storage pool. If so, check if ACP allows it.
	if pool.InternalAttributes()[Kerberos] != "" {
		if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption); err != nil {
			Logc(ctx).WithField(
				"feature", acp.FeatureInflightEncryption,
			).WithError(err).Errorf("Failed to create volume.")
			return fmt.Errorf("feature %s requires ACP; %w", acp.FeatureInflightEncryption, err)
		}
	}

	// If the volume already exists, bail out
	volumeExists, extantVolume, err := d.SDK.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s; %v", name, err)
	}
	if volumeExists {
		if extantVolume.ProvisioningState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}

		Logc(ctx).WithFields(LogFields{
			"name":  name,
			"state": extantVolume.ProvisioningState,
		}).Warning("Volume already exists.")

		// Volume here is likely in terminal state. Get the exact error message why is it so.
		if err = d.waitForVolumeCreate(ctx, extantVolume, api.Create); err != nil {
			return err
		}

		// No specific error is returned, so return a generic volume exists error
		return drivers.NewVolumeExistsError(name)
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s; %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil || sizeBytes > math.MaxInt64 { // the azure api requires both int64 and uint64
		return fmt.Errorf("%v is an invalid volume size; %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := capacity.ToBytes(pool.InternalAttributes()[Size])
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}

	// Enforce minimum volume size
	if sizeBytes < MinimumANFVolumeSizeBytes {

		Logc(ctx).WithFields(LogFields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (50 GiB).")

		sizeBytes = MinimumANFVolumeSizeBytes
	}

	// Ensure sizeBytes is a multiple of 1 GiB
	if remainder := sizeBytes % capacity.OneGiB; remainder != 0 {
		sizeBytes = ((sizeBytes / capacity.OneGiB) + 1) * capacity.OneGiB
	}

	if _, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	serviceLevel := convert.ToTitle(volConfig.ServiceLevel)
	if serviceLevel == "" {
		serviceLevel = pool.InternalAttributes()[ServiceLevel]
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
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

	// Take kerberos option from pool
	kerberos := pool.InternalAttributes()[Kerberos]

	// Take QOS type from pool
	qosType := pool.InternalAttributes()[QOSType]

	// Determine max throughput for manual QOS pools
	maxThroughput := ""
	var maxThroughputPtr *float32
	if qosType == api.QOSManual {
		maxThroughput = pool.InternalAttributes()[MaxThroughput]
		parsedMaxThroughput, err := strconv.ParseFloat(maxThroughput, 32)
		if err != nil {
			return fmt.Errorf("invalid value for maxThroughput: %v", err)
		}
		maxThroughputPtr = convert.ToPtr(float32(parsedMaxThroughput))
	}

	// Determine protocol from mount options
	var protocolTypes []string
	var cifsAccess, nfsV3Access, nfsV41Access, kerberosEnabled bool
	var apiExportRule api.ExportRule
	var exportPolicy api.ExportPolicy

	switch kerberos {
	case api.MountOptionKerberos5, api.MountOptionKerberos5I, api.MountOptionKerberos5P:
		kerberosEnabled = true
	case "":
		kerberosEnabled = false
	default:
		return fmt.Errorf("unsupported kerberos type: %s", kerberos)
	}

	var nfsVersion string
	if d.Config.NASType == sa.SMB {
		protocolTypes = []string{api.ProtocolTypeCIFS}
	} else {
		nfsVersion, err = nfs.GetNFSVersionFromMountOptions(mountOptions, nfsVersion3, supportedNFSVersions)
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

		apiExportRule = api.ExportRule{
			AllowedClients: pool.InternalAttributes()[ExportRule],
			Cifs:           cifsAccess,
			Nfsv3:          nfsV3Access,
			Nfsv41:         nfsV41Access,
			RuleIndex:      1,
			UnixReadOnly:   false,
			UnixReadWrite:  true,
		}

		if kerberosEnabled {
			protocolTypes = []string{api.ProtocolTypeNFSv41}
			apiExportRule.Nfsv3 = false
			apiExportRule.Nfsv41 = true
			apiExportRule.UnixReadOnly = false
			apiExportRule.UnixReadWrite = false

			switch kerberos {
			case api.MountOptionKerberos5:
				apiExportRule.Kerberos5ReadWrite = true
			case api.MountOptionKerberos5I:
				apiExportRule.Kerberos5IReadWrite = true
			case api.MountOptionKerberos5P:
				apiExportRule.Kerberos5PReadWrite = true
			}
		}

		exportPolicy = api.ExportPolicy{
			Rules: []api.ExportRule{apiExportRule},
		}
	}

	// Set snapshot directory from volume config first (handles Docker case), then from pool
	// If none is set, set it based on mountOption by default; for nfsv3 => false, nfsv4/4.1 => true
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes()[SnapshotDir]
		// If snapshot directory is not set at pool level, then set default value based on mount option
		if snapshotDir == "" {
			if strings.HasPrefix(nfsVersion, "4") {
				snapshotDir = "true"
			} else {
				snapshotDir = "false"
			}
		}
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir; %v", err)
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = d.getTelemetryLabels(ctx)

	poolLabels, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	if err != nil {
		return err
	}
	labels[storage.ProvisioningLabelTag] = poolLabels

	networkFeatures := pool.InternalAttributes()[NetworkFeatures]

	// Get topology, if any, and determine the matched Azure zone.  If the pool has no zones, assume none are in use.
	var region, zone string
	if len(pool.SupportedTopologies()) > 0 {
		if topology, topologyErr := sc.GetTopologyForVolume(ctx, volConfig, pool); topologyErr != nil {
			return topologyErr
		} else {
			region, zone = sc.GetRegionZoneForTopology(topology)
			zone, _ = strings.CutPrefix(zone, region+"-")
		}
	}

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	volConfig.ServiceLevel = serviceLevel
	volConfig.SnapshotDir = strconv.FormatBool(snapshotDirBool)
	volConfig.UnixPermissions = unixPermissions
	volConfig.Zone = zone

	// Find a subnet
	subnet := d.SDK.RandomSubnetForStoragePool(ctx, pool)
	if subnet == nil {
		return fmt.Errorf("no subnets found for storage pool %s", pool.Name())
	}

	// Find matching capacity pools
	cPools := d.SDK.CapacityPoolsForStoragePool(ctx, pool, serviceLevel, qosType)
	if len(cPools) == 0 {
		return fmt.Errorf("no capacity pools found for storage pool %s", pool.Name())
	}

	createErrors := multierr.Combine()

	// Try each capacity pool until one works
	for _, cPool := range cPools {
		keyVaultEndpointID := ""
		if keyVaultEndpoint, ok := d.Config.CustomerEncryptionKeys[cPool.NetAppAccount]; ok {
			if networkFeatures == "" {
				networkFeatures = api.NetworkFeaturesStandard
			}

			keyVaultEndpointID = api.CreateKeyVaultEndpoint(d.Config.SubscriptionID, cPool.ResourceGroup,
				keyVaultEndpoint)
		}

		if d.Config.NASType == sa.SMB {
			Logc(ctx).WithFields(LogFields{
				"capacityPool":     cPool.Name,
				"creationToken":    name,
				"size":             sizeBytes,
				"serviceLevel":     serviceLevel,
				"snapshotDir":      snapshotDirBool,
				"protocolTypes":    protocolTypes,
				"networkFeatures":  networkFeatures,
				"keyVaultEndpoint": keyVaultEndpointID,
				"zone":             zone,
				"maxThroughput":    maxThroughput,
			}).Debug("Creating volume.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"capacityPool":     cPool.Name,
				"creationToken":    name,
				"size":             sizeBytes,
				"unixPermissions":  unixPermissions,
				"serviceLevel":     serviceLevel,
				"snapshotDir":      snapshotDirBool,
				"protocolTypes":    protocolTypes,
				"exportPolicy":     fmt.Sprintf("%+v", exportPolicy),
				"networkFeatures":  networkFeatures,
				"keyVaultEndpoint": keyVaultEndpointID,
				"zone":             zone,
				"maxThroughput":    maxThroughput,
			}).Debug("Creating volume.")
		}

		createRequest := &api.FilesystemCreateRequest{
			ResourceGroup:      cPool.ResourceGroup,
			NetAppAccount:      cPool.NetAppAccount,
			CapacityPool:       cPool.Name,
			Name:               volConfig.Name,
			SubnetID:           subnet.ID,
			CreationToken:      name,
			Labels:             labels,
			ProtocolTypes:      protocolTypes,
			QuotaInBytes:       int64(sizeBytes),
			SnapshotDirectory:  snapshotDirBool,
			NetworkFeatures:    networkFeatures,
			KerberosEnabled:    kerberosEnabled,
			KeyVaultEndpointID: keyVaultEndpointID,
			Zone:               zone,
			MaxThroughput:      maxThroughputPtr,
		}

		// Add unix permissions and export policy fields only to NFS volume
		if d.Config.NASType == sa.NFS {
			createRequest.UnixPermissions = unixPermissions
			createRequest.ExportPolicy = exportPolicy
		}

		// Create the volume
		volume, createErr := d.SDK.CreateVolume(ctx, createRequest)
		if createErr != nil {
			errMessage := fmt.Sprintf("ANF pool %s; error creating volume %s: %v", cPool.Name, name, createErr)
			Logc(ctx).Error(errMessage)
			createErrors = multierr.Combine(createErrors, errors.New(errMessage))
			continue
		}

		// Always save the ID so we can find the volume efficiently later
		volConfig.InternalID = volume.ID

		// Wait for creation to complete so that the mount targets are available
		return d.waitForVolumeCreate(ctx, volume, api.Create)
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

	// If source volume has kerberos enabled, check if ACP allows it.
	if sourceVolume.KerberosEnabled {
		if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption); err != nil {
			Logc(ctx).WithField(
				"feature", acp.FeatureInflightEncryption,
			).WithError(err).Errorf("Failed to clone volume.")
			return fmt.Errorf("feature %s requires ACP; %w", acp.FeatureInflightEncryption, err)
		}
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
			return errors.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}

		// Volume here is likely in terminal state. Get the exact error message why is it so.
		if err = d.waitForVolumeCreate(ctx, extantVolume, api.Create); err != nil {
			return err
		}

		// No specific error is returned, so return a generic volume exists error
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

		// Save the snapshot name in the volume config, so we can auto-delete it later
		cloneVolConfig.CloneSourceSnapshotInternal = sourceSnapshot.Name

		Logc(ctx).WithFields(LogFields{
			"snapshot": sourceSnapshot.Name,
			"source":   sourceVolume.Name,
		}).Info("Created source snapshot.")
	}

	// If RO clone is requested, don't create the volume on ANF backend and return nil
	if cloneVolConfig.ReadOnlyClone {
		// Return error , if snapshot directory is not enabled for RO clone
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
		poolLabels, err := storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
		if err != nil {
			return err
		}

		labels[storage.ProvisioningLabelTag] = poolLabels
	}

	keyVaultEndpointID := sourceVolume.KeyVaultEndpointID
	networkFeatures := sourceVolume.NetworkFeatures
	if _, ok := d.Config.CustomerEncryptionKeys[sourceVolume.NetAppAccount]; ok {
		if networkFeatures == api.NetworkFeaturesBasic {
			// Customer encryption keys are passed but the network feature is set to Basic.
			Logc(ctx).WithFields(LogFields{
				"snapshot": sourceSnapshot.Name,
				"source":   sourceVolume.Name,
			}).Debug("Source volume's network feature is set to Basic. Hence, " +
				"falling back to volume encryption with Microsoft managed keys.")
		} else {
			networkFeatures = api.NetworkFeaturesStandard
		}
	}

	zone := ""
	if len(sourceVolume.Zones) > 0 {
		zone = sourceVolume.Zones[0]
	}

	var maxThroughputPtr *float32
	if sourceVolume.MaxThroughput > 0 {
		maxThroughputPtr = &sourceVolume.MaxThroughput
	}

	Logc(ctx).WithFields(LogFields{
		"creationToken":   name,
		"sourceVolume":    sourceVolume.CreationToken,
		"sourceSnapshot":  sourceSnapshot.Name,
		"unixPermissions": sourceVolume.UnixPermissions,
		"networkFeatures": networkFeatures,
		"zone":            zone,
		"maxThroughput":   sourceVolume.MaxThroughput,
	}).Debug("Cloning volume.")

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:      sourceVolume.ResourceGroup,
		NetAppAccount:      sourceVolume.NetAppAccount,
		CapacityPool:       sourceVolume.CapacityPool,
		Name:               cloneVolConfig.Name,
		SubnetID:           sourceVolume.SubnetID,
		CreationToken:      name,
		Labels:             labels,
		ProtocolTypes:      sourceVolume.ProtocolTypes,
		QuotaInBytes:       sourceVolume.QuotaInBytes,
		SnapshotDirectory:  sourceVolume.SnapshotDirectory,
		SnapshotID:         sourceSnapshot.ID,
		NetworkFeatures:    networkFeatures,
		KeyVaultEndpointID: keyVaultEndpointID,
		Zone:               zone,
		MaxThroughput:      maxThroughputPtr,
	}

	// Add unix permissions and export policy fields only to NFS volume
	if d.Config.NASType == sa.NFS {
		createRequest.ExportPolicy = sourceVolume.ExportPolicy
		createRequest.UnixPermissions = sourceVolume.UnixPermissions
		createRequest.KerberosEnabled = sourceVolume.KerberosEnabled
	}

	// Clone the volume
	clone, err := d.SDK.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Always save the ID so we can find the volume efficiently later
	cloneVolConfig.InternalID = clone.ID

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, clone, api.Create)
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
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.VolumeByCreationToken(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", originalName, err)
	}

	// Don't allow import for dual-protocol volume.
	// For dual-protocol volume the ProtocolTypes has two values [NFSv3, CIFS]
	if len(volume.ProtocolTypes) > 1 {
		return fmt.Errorf("trident doesn't support importing a dual-protocol volume '%s'", originalName)
	}

	// Ensure the volume may be imported by a capacity pool managed by this backend
	if err = d.SDK.EnsureVolumeInValidCapacityPool(ctx, volume); err != nil {
		return err
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	Logc(ctx).WithFields(LogFields{
		"creationToken": volume.CreationToken,
		"managed":       !volConfig.ImportNotManaged,
		"state":         volume.ProvisioningState,
		"capacityPool":  volume.CapacityPool,
		"sizeBytes":     volume.QuotaInBytes,
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

		// Check for kerberos option from backend config
		kerberos := d.Config.Kerberos
		if kerberos != "" {
			// Check if this volume landed on a Kerberos-enabled storage pool. If so, check if ACP allows it.
			if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption); err != nil {
				Logc(ctx).WithField(
					"feature", acp.FeatureInflightEncryption,
				).WithError(err).Errorf("Could not import volume.")
				return fmt.Errorf("feature %s requires ACP; %w", acp.FeatureInflightEncryption, err)
			}
		}

		if kerberos != "" && !volume.KerberosEnabled {
			return fmt.Errorf("could not import non-kerberos volume '%s', on a kerberos enabled backend", originalName)
		}

		if kerberos == "" && volume.KerberosEnabled {
			return fmt.Errorf("could not import kerberos volume '%s', on a non-kerberos enabled backend", originalName)
		}

		modifiedExportRule := api.ExportRule{}
		switch kerberos {
		case api.MountOptionKerberos5:
			modifiedExportRule.Nfsv41 = true
			modifiedExportRule.Kerberos5ReadWrite = true
		case api.MountOptionKerberos5I:
			modifiedExportRule.Nfsv41 = true
			modifiedExportRule.Kerberos5IReadWrite = true
		case api.MountOptionKerberos5P:
			modifiedExportRule.Nfsv41 = true
			modifiedExportRule.Kerberos5PReadWrite = true
		}

		// Update the volume labels
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, volume.Labels[storage.ProvisioningLabelTag]) {
			volume.Labels[storage.ProvisioningLabelTag] = ""
		}
		labels := d.updateTelemetryLabels(ctx, volume)

		if d.Config.NASType == sa.SMB && volume.ProtocolTypes[0] == api.ProtocolTypeCIFS {
			if err = d.SDK.ModifyVolume(ctx, volume, labels, nil, &snapshotDirAccess, &modifiedExportRule); err != nil {
				Logc(ctx).WithField("originalName", originalName).WithError(err).Error(
					"Could not import volume, volume modify failed.")
				return fmt.Errorf("could not import volume %s, volume modify failed; %v", originalName, err)
			}

			Logc(ctx).WithFields(LogFields{
				"name":          volume.Name,
				"creationToken": volume.CreationToken,
				"labels":        labels,
			}).Info("Volume modified.")

		} else if d.Config.NASType == sa.NFS && (volume.ProtocolTypes[0] == api.ProtocolTypeNFSv3 || volume.
			ProtocolTypes[0] == api.ProtocolTypeNFSv41) {
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

			err = d.SDK.ModifyVolume(ctx, volume, labels, &unixPermissions, &snapshotDirAccess, &modifiedExportRule)
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

		if _, err = d.SDK.WaitForVolumeState(
			ctx, volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout(), api.Import); err != nil {
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
	// ANF volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

// getTelemetryLabels builds the standard telemetry labels that are set on each volume.
func (d *NASStorageDriver) getTelemetryLabels(ctx context.Context) string {
	telemetry := map[string]Telemetry{drivers.TridentLabelTag: *d.telemetry}

	telemetryJSON, err := json.Marshal(telemetry)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal telemetry: %+v", telemetry)
	}

	return strings.ReplaceAll(string(telemetryJSON), " ", "")
}

// updateTelemetryLabels updates a volume's labels to include the standard telemetry labels.
func (d *NASStorageDriver) updateTelemetryLabels(ctx context.Context, volume *api.FileSystem) map[string]string {
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
func (d *NASStorageDriver) waitForVolumeCreate(
	ctx context.Context, volume *api.FileSystem, operation api.Operation,
) error {
	state, err := d.SDK.WaitForVolumeState(
		ctx, volume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout, operation)
	if err != nil {

		logFields := LogFields{"volume": volume.CreationToken}

		switch state {

		case api.StateAccepted, api.StateCreating:
			Logc(ctx).WithFields(logFields).Debugf("Volume is in %s state.", state)
			return errors.VolumeCreatingError(err.Error())

		case api.StateDeleting:
			// Don't wait if volume is already being deleted
			Logc(ctx).WithFields(logFields).WithError(err).Error(
				"Volume is being cleaned up and should be recreated later.")

		case api.StateError:
			// Delete a failed volume
			if errDelete := d.SDK.DeleteVolume(ctx, volume); errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Volume could not be cleaned up and must be manually deleted.")
				return multierr.Combine(err, errDelete)
			} else {
				Logc(ctx).WithField("volume", volume.Name).Info("Cleanup of failed volume started.")
			}

			Logc(ctx).WithFields(logFields).Debugf("Volume is in %s state.", state)
			return err

		case api.StateMoving, api.StateReverting:
			fallthrough

		default:
			Logc(ctx).WithFields(logFields).Errorf("unexpected volume state %s found for volume", state)
		}
	}

	return err
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
	if err = d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	hasAutomaticSnapshot := false

	// If this volume was cloned from an automatic snapshot, delete the snapshot after deleting the volume.
	if volConfig.CloneSourceSnapshot == "" && volConfig.CloneSourceSnapshotInternal != "" {
		hasAutomaticSnapshot = true
		defer func() {
			d.deleteAutomaticSnapshot(ctx, err, volConfig)
		}()
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
		// This is a retry, so give more time before giving up again. Only do this if the context is Docker or
		// if the CSI volume was created out of an automatic snapshot. Don't wait in other contexts.
		if d.Config.DriverContext == tridentconfig.ContextDocker || hasAutomaticSnapshot {
			_, err = d.SDK.WaitForVolumeState(
				ctx, extantVolume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout(), api.Delete)
			return err
		}
		return nil
	}

	// Delete the volume
	if err = d.SDK.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	Logc(ctx).WithField("volume", extantVolume.Name).Info("Volume deleted.")

	// If Docker or if the CSI volume was created out of an automatic snapshot, wait for volume deletion to complete.
	// Don't wait in other contexts.
	if d.Config.DriverContext == tridentconfig.ContextDocker || hasAutomaticSnapshot {
		_, err = d.SDK.WaitForVolumeState(
			ctx, extantVolume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout(), api.Delete)
		return err
	}
	return nil
}

// deleteAutomaticSnapshot deletes a snapshot that was created automatically during volume clone creation.
// An automatic snapshot is detected by the presence of CloneSourceSnapshotInternal in the volume config
// while CloneSourceSnapshot is not set.  This method is called after the volume has been deleted, and it
// will only attempt snapshot deletion if the clone volume deletion completed without error.  This is a
// best-effort method, and any errors encountered will be logged but not returned.
func (d *NASStorageDriver) deleteAutomaticSnapshot(
	ctx context.Context, volDeleteError error, cloneVolConfig *storage.VolumeConfig,
) {
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal
	cloneSourceName := cloneVolConfig.CloneSourceVolumeInternal
	cloneName := cloneVolConfig.InternalName

	fields := LogFields{
		"Method":          "DeleteSnapshot",
		"Type":            "NASStorageDriver",
		"snapshotName":    snapshot,
		"cloneSourceName": cloneSourceName,
		"cloneName":       cloneName,
	}

	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> deleteAutomaticSnapshot")
	defer Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< deleteAutomaticSnapshot")

	logFields := LogFields{
		"snapshotName":    snapshot,
		"cloneSourceName": cloneSourceName,
		"cloneName":       cloneName,
	}

	// Check if there is anything to do
	if !(cloneVolConfig.CloneSourceSnapshot == "" && cloneVolConfig.CloneSourceSnapshotInternal != "") {
		Logc(ctx).WithFields(logFields).Debug("No automatic clone source snapshot existed, skipping cleanup.")
		return
	}

	// If the clone volume couldn't be deleted, don't attempt to delete any automatic snapshot.
	if volDeleteError != nil {
		Logc(ctx).WithFields(logFields).Debug("Error deleting volume, skipping automatic snapshot cleanup.")
		return
	}

	// We always clone to the same capacity pool, so we can use the clone volume's info to construct
	// the source volume ID.
	_, resourceGroup, _, netappAccount, cPoolName, _, err := api.ParseVolumeID(cloneVolConfig.InternalID)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error parsing clone volume ID")
	}

	cloneSourceID := api.CreateVolumeID(d.Config.SubscriptionID, resourceGroup, netappAccount,
		cPoolName, cloneSourceName)

	// Get the volume
	sourceVolume, err := d.SDK.VolumeByID(ctx, cloneSourceID)
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
	sourceSnapshot, err := d.SDK.SnapshotForVolume(ctx, sourceVolume, snapshot)
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
	if err = d.SDK.DeleteSnapshot(ctx, sourceVolume, sourceSnapshot); err != nil {
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
	var volume *api.FileSystem
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
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// If it's a RO clone, get source volume to populate publish info
	if volConfig.ReadOnlyClone {
		volume, err = d.SDK.VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	} else {
		// Get the volume
		volume, err = d.SDK.Volume(ctx, volConfig)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}

		// Heal the ID on legacy volumes
		if volConfig.InternalID == "" {
			volConfig.InternalID = volume.ID
		}
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add required fields for attaching SMB volume
	if d.Config.NASType == sa.SMB {
		publishInfo.SMBPath = volConfig.AccessInfo.SMBPath
		publishInfo.SMBServer = (volume.MountTargets)[0].ServerFqdn
		publishInfo.FilesystemType = sa.SMB
	} else {
		// Add fields needed by Attach
		publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
		publishInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
		publishInfo.FilesystemType = sa.NFS
		publishInfo.MountOptions = mountOptions
	}

	// Replace server IP with FQDN for kerberos volume
	if volume.KerberosEnabled {
		publishInfo.NfsServerIP = (volume.MountTargets)[0].ServerFqdn
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
		if errors.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}

	if snapshot.ProvisioningState != api.StateAvailable {
		return nil, fmt.Errorf("snapshot %s state is %s", internalSnapName, snapshot.ProvisioningState)
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
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", internalVolName, err)
	}

	// Get the snapshot
	snapshot, err := d.SDK.SnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	// Do the restore
	if err = d.SDK.RestoreSnapshot(ctx, volume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	_, err = d.SDK.WaitForVolumeState(ctx, volume, api.StateAvailable,
		[]string{api.StateError, api.StateDeleting, api.StateDeleted}, api.DefaultSDKTimeout, api.Restore,
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
		if errors.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("unable to find snapshot %s; %v", internalSnapName, err)
	}

	if err = d.SDK.DeleteSnapshot(ctx, extantVolume, snapshot); err != nil {
		return err
	}

	// If Docker, wait for deletion to complete.  Don't wait in other contexts.
	if d.Config.DriverContext == tridentconfig.ContextDocker {
		return d.SDK.WaitForSnapshotState(
			ctx, snapshot, extantVolume, api.StateDeleted, []string{api.StateError}, api.SnapshotTimeout,
		)
	}
	return nil
}

// List returns the list of volumes associated with this backend.
func (d *NASStorageDriver) List(ctx context.Context) ([]string, error) {
	fields := LogFields{"Method": "List", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> List")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< List")

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
func (d *NASStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

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

	if sizeBytes > math.MaxInt64 {
		Logc(ctx).WithFields(fields).Error("Invalid volume size.")
		return errors.New("invalid volume size")
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

	// Ensure sizeBytes is a multiple of 1 GiB
	if remainder := sizeBytes % capacity.OneGiB; remainder != 0 {
		sizeBytes = ((sizeBytes / capacity.OneGiB) + 1) * capacity.OneGiB
	}

	// Resize the volume
	if err = d.SDK.ResizeVolume(ctx, volume, int64(sizeBytes)); err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
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
func (d *NASStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.ANFStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NASStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. Resource Group - contains at least one netapp account; names are unique within a subscription.
	// 2. NetappAccount - contains at least one capacity pool; names are unique to resource group OR region;
	//   names can only be reused if resource group AND region differ from matching account name.
	// 3. Capacity Pool - names are unique within a given ResourceGroup and NetappAccount.

	// CapacityPoolsForStoragePools relies on an internal mapping of storage pools created from the driver config.
	// If the behavior of that method should ever change, this method will need to change as well.
	cPools := d.SDK.CapacityPoolsForStoragePools(ctx)
	backendPools := make([]drivers.ANFStorageBackendPool, 0, len(cPools))
	for _, cPool := range cPools {
		backendPools = append(backendPools, drivers.ANFStorageBackendPool{
			SubscriptionID: d.Config.SubscriptionID,
			ResourceGroup:  cPool.ResourceGroup,
			NetappAccount:  cPool.NetAppAccount,
			Location:       cPool.Location,
			CapacityPool:   cPool.Name,
		})
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
		// UUID-based name with a prefix that won't exceed the 36-character limit.
		return "anf-" + uuid.NewString()
	}
}

// CreateFollowup is called after volume creation and sets the access info in the volume config.
func (d *NASStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	var volume *api.FileSystem
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
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	// If it's a RO clone, get source volume to populate access details
	if volConfig.ReadOnlyClone {
		volume, err = d.SDK.VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	} else {
		// Get the volume
		volume, err = d.SDK.Volume(ctx, volConfig)
		if err != nil {
			return fmt.Errorf("could not find volume %s; %v", name, err)
		}
	}

	// Ensure volume is in a good state
	if volume.ProvisioningState != api.StateAvailable {
		return fmt.Errorf("volume %s is in %s state, not %s", name, volume.ProvisioningState, api.StateAvailable)
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volConfig.InternalName)
	}

	// Set the mount target based on the NASType
	if d.Config.NASType == sa.SMB {
		volConfig.AccessInfo.SMBPath = constructVolumeAccessPath(volConfig, volume, sa.SMB)
		volConfig.AccessInfo.SMBServer = (volume.MountTargets)[0].ServerFqdn
		volConfig.FileSystem = sa.SMB
	} else {
		volConfig.AccessInfo.NfsPath = constructVolumeAccessPath(volConfig, volume, sa.NFS)
		volConfig.AccessInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
		volConfig.FileSystem = sa.NFS
	}

	// Replace server IP with FQDN for kerberos volume
	if volume.KerberosEnabled {
		volConfig.AccessInfo.NfsServerIP = (volume.MountTargets)[0].ServerFqdn
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
	b.AzureConfig = &d.Config
}

// GetExternalConfig returns a clone of this backend's config, sanitized for external consumption.
func (d *NASStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AzureNASStorageDriverConfig
	drivers.Clone(ctx, d.Config, &cloneConfig)
	cloneConfig.ClientSecret = tridentconfig.REDACTED // redact the Secret
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: tridentconfig.REDACTED,
		drivers.KeyType: tridentconfig.REDACTED,
	} // redact the credentials
	return cloneConfig
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the unique creation
// token used when creating the volume.
func (d *NASStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		return nil, fmt.Errorf("could not update ANF resource cache; %v", err)
	}

	filesystem, err := d.SDK.VolumeByCreationToken(ctx, volumeID)
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
func (d *NASStorageDriver) getVolumeExternal(volumeAttrs *api.FileSystem) *storage.VolumeExternal {
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

// validateStoragePrefix ensures the storage prefix is valid
func validateStoragePrefix(storagePrefix string) error {
	if !storagePrefixRegex.MatchString(storagePrefix) {
		return fmt.Errorf("storage prefix may only contain letters and hyphens and must begin with a letter")
	}
	return nil
}

// validateNetworkFeatures ensures that networking is valid when customer encryption keys are passed.
func validateNetworkFeatures(networking string, encryptionKeys map[string]string) error {
	if len(encryptionKeys) > 0 && networking == api.NetworkFeaturesBasic {
		return fmt.Errorf("customer encryption keys are not supported with Basic networking")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d *NASStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func constructVolumeAccessPath(
	volConfig *storage.VolumeConfig, volume *api.FileSystem, protocol string,
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

func (d *NASStorageDriver) Update(
	ctx context.Context, volConfig *storage.VolumeConfig,
	updateInfo *models.VolumeUpdateInfo, allVolumes map[string]*storage.Volume,
) (map[string]*storage.Volume, error) {
	name := volConfig.Name
	fields := LogFields{
		"Method":       "Update",
		"Type":         "AzureNASStorageDriver",
		"name":         name,
		"internalName": volConfig.InternalName,
		"updateInfo":   updateInfo,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Update")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Update")

	updateErrorMsg := fmt.Sprintf("Failed to update volume %v.", name)

	// Ensure there is something to update
	if updateInfo == nil {
		msg := fmt.Sprintf("nothing to update for volume %v", name)
		err := errors.InvalidInputError(msg)
		Logc(ctx).WithError(err).Error(updateErrorMsg)
		return nil, err
	}

	// Update resource cache as needed
	if err := d.SDK.RefreshAzureResources(ctx); err != nil {
		updateErr := fmt.Errorf("could not update ANF resource cache; %v", err)
		Logc(ctx).WithError(updateErr).Error(updateErrorMsg)
		return nil, updateErr
	}

	// Get the volume
	volume, err := d.SDK.Volume(ctx, volConfig)
	if err != nil {
		updateErr := fmt.Errorf("could not find volume %s; %v", name, err)
		Logc(ctx).WithError(updateErr).Error(updateErrorMsg)
		return nil, updateErr
	}

	// Heal the ID on legacy volumes
	if volConfig.InternalID == "" {
		volConfig.InternalID = volume.ID
	}

	// If the volume state isn't Available, return an error
	if volume.ProvisioningState != api.StateAvailable {
		updateErr := fmt.Errorf("volume %s state is %s, not %s", name, volume.ProvisioningState, api.StateAvailable)
		Logc(ctx).WithError(updateErr).Error(updateErrorMsg)
		return nil, updateErr
	}

	var updatedVols map[string]*storage.Volume
	var updateError error

	// Update snapshotDirectory for volume
	if updateInfo.SnapshotDirectory != "" {
		updatedVols, updateError = d.updateSnapshotDirectory(ctx, name, volume, updateInfo.SnapshotDirectory,
			allVolumes)
	}

	return updatedVols, updateError
}

func (d *NASStorageDriver) updateSnapshotDirectory(
	ctx context.Context, name string, volume *api.FileSystem,
	snapshotDir string, allVolumes map[string]*storage.Volume,
) (map[string]*storage.Volume, error) {
	fields := LogFields{
		"Method":      "updateSnapshotDirectory",
		"Type":        "AzureNASStorageDriver",
		"name":        name,
		"snapshotDir": snapshotDir,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> updateSnapshotDirectory")
	defer Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< updateSnapshotDirectory")

	genericLogError := fmt.Sprintf("Failed to update snapshot directory for volume %v.", name)

	// Ensure ACP is enabled
	if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureReadOnlyClone); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error(genericLogError)
		return nil, err
	}

	// Validate request
	snapDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		failureErr := errors.InvalidInputError(fmt.Sprintf("invalid value for snapshot directory %v; %v", snapshotDir,
			err))
		Logc(ctx).WithError(failureErr).Error(genericLogError)
		return nil, failureErr
	}

	// Modify volume snapshotDirectory if the current value is different from requested
	if volume.SnapshotDirectory != snapDirBool {
		if err = d.SDK.ModifyVolume(ctx, volume, nil, nil, &snapDirBool, nil); err != nil {
			Logc(ctx).WithError(err).Error(genericLogError)
			return nil, err
		}
	}

	// Update volConfig for the volume to ensure cache is appropriately updated
	// Always do this, to ensure cache is always in consistent state with the requested value
	// There could be a case where snapshotDirectory is modified directly in the backend and trident cache is not updated
	var vol *storage.Volume
	if vol = allVolumes[name]; vol == nil {
		failureErr := fmt.Errorf("volume %v not found", name)
		Logc(ctx).WithError(failureErr).Error(genericLogError)
		return nil, failureErr
	}
	vol.Config.SnapshotDir = strconv.FormatBool(snapDirBool)

	return map[string]*storage.Volume{name: vol}, nil
}
