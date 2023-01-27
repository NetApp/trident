// Copyright 2021 NetApp, Inc. All Rights Reserved.

package azure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumSubvolumeSizeBytes = uint64(20971520) // 20 MB
	RequiredHashLength        = 16

	defaultSubvolumeSizeStr = "20971520"

	snapshotNameSeparator = "--"
	pvcPrefix             = "pvc-"
)

var (
	subvolumeNameRegex          = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,39}$`)
	subvolumeSnapshotNameRegex  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,44}$`)
	subvolumeCreationTokenRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,63}$`)

	pollerResponseCache = make(map[PollerKey]api.PollerResponse)
)

type PollerKey struct {
	ID        string
	Operation string
}

type SubvolumeHelper struct {
	Config         drivers.AzureNASStorageDriverConfig
	Context        tridentconfig.DriverContext
	SnapshotRegexp *regexp.Regexp
}

func NewFileHelper(config drivers.AzureNASStorageDriverConfig, context tridentconfig.DriverContext) *SubvolumeHelper {
	subvolumeHelper := SubvolumeHelper{}
	subvolumeHelper.Config = config
	subvolumeHelper.Context = context
	regexSnapshotName := fmt.Sprintf("(?m)%v-(.+?)%v(.+)", *subvolumeHelper.Config.StoragePrefix, snapshotNameSeparator)
	subvolumeHelper.SnapshotRegexp = regexp.MustCompile(regexSnapshotName)

	return &subvolumeHelper
}

// volName is expected not to have the storage prefix included
// parameters: volName=pvc-abc1234-324abc34
// output: abc12
func (o *SubvolumeHelper) GetSnapshotSuffix(volName string) string {
	var suffix string
	if strings.HasPrefix(volName, pvcPrefix) && len(volName) > len(pvcPrefix) {
		uid := strings.Split(volName, pvcPrefix)[1]
		suffix = uid[0:5]
	} else if len(volName) > 5 {
		suffix = volName[0:4]
	} else {
		suffix = volName
	}

	return suffix
}

// volName is expected not to have the storage prefix included
// parameters: volName=pvc-abc1234-324abc34 snapName=my-Snapshot
// output: prefix-my-Snapshot--abc12
func (o *SubvolumeHelper) GetSnapshotInternalName(volName, snapNameValue string) string {
	snapName := strings.Replace(snapNameValue, snapshotNameSeparator, "-", -1)

	name := fmt.Sprintf("%v-%v%v%v", *o.Config.StoragePrefix, snapName, snapshotNameSeparator,
		o.GetSnapshotSuffix(volName))

	return name
}

func (o *SubvolumeHelper) getSnapshotInternalNameComponents(snapshotInternalName string) []string {
	result := o.SnapshotRegexp.FindStringSubmatch(snapshotInternalName)
	// result [0] is the full string: prefix-mySnap--chars
	// result [1] is the snapshot name: mySnap
	// result [2] is chars (unused)
	return result
}

// parameter: snapshotInternalName=storagePrefix-mySnap_chars
// result [1] is the snapshot name: mySnap
func (o *SubvolumeHelper) GetSnapshotNameFromSnapInternalName(snapshotInternalName string) string {
	result := o.getSnapshotInternalNameComponents(snapshotInternalName)
	if len(result) > 2 {
		return result[1]
	}
	return ""
}

// parameter: snapshotInternalName=storagePrefix-mySnap_chars
// result [2] is the snapshot suffix: chars
func (o *SubvolumeHelper) GetSnapshotSuffixFromSnapshotInternalName(snapshotInternalName string) string {
	result := o.getSnapshotInternalNameComponents(snapshotInternalName)
	if len(result) > 2 {
		return result[2]
	}
	return ""
}

// IsValidSnapshotInternalName identifies if the given snapLunPath has a valid snapshot name
func (o *SubvolumeHelper) IsValidSnapshotInternalName(snapshotInternalName string) bool {
	snapshotName := o.GetSnapshotNameFromSnapInternalName(snapshotInternalName)
	return snapshotName != ""
}

// NASBlockStorageDriver is for storage provisioning using the Azure NetApp Files service.
type NASBlockStorageDriver struct {
	initialized         bool
	Config              drivers.AzureNASStorageDriverConfig
	telemetry           *Telemetry
	SDK                 api.Azure
	helper              *SubvolumeHelper
	volumeCreateTimeout time.Duration

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool
}

// Name returns the name of this driver.
func (d *NASBlockStorageDriver) Name() string {
	return tridentconfig.AzureNASBlockStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance.
func (d *NASBlockStorageDriver) defaultBackendName() string {
	id := utils.RandomString(6)
	if len(d.Config.ClientID) > 5 {
		id = d.Config.ClientID[0:5]
	}
	return fmt.Sprintf("%s_%s", strings.Replace(d.Name(), "-", "", -1), id)
}

// BackendName returns the name of the backend managed by this driver instance.
func (d *NASBlockStorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		// Use the old naming scheme if no name is specified
		return d.defaultBackendName()
	}
}

// poolName constructs the name of the pool reported by this driver instance.
func (d *NASBlockStorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// validateVolumeName checks that the name of a new volume matches the requirements of an ANF subvolume name.
func (d *NASBlockStorageDriver) validateVolumeName(name string) error {
	if !subvolumeNameRegex.MatchString(name) {
		return fmt.Errorf("subvolume name '%s' is not allowed; it must be 1-40 characters long, "+
			"begin with a character, and contain only characters, digits, and hyphens", name)
	} else if strings.Contains(name, api.SubvolumeNameSeparator) {
		return fmt.Errorf("subvolume name '%s' should not contains '%s' pattern", name,
			api.SubvolumeNameSeparator)
	}

	return nil
}

// validateSnapshotName checks that the name of a new snapshot matches the requirements of an ANF subvolume
// snapshot name.
func (d *NASBlockStorageDriver) validateSnapshotName(name string) error {
	if !subvolumeSnapshotNameRegex.MatchString(name) {
		return fmt.Errorf("subvolume name '%s' is not allowed; it must be 1-45 characters long, "+
			"begin with a character, and contain only characters, digits, and hyphens", name)
	} else if strings.Contains(name, snapshotNameSeparator) {
		return fmt.Errorf("snapshot name '%s' should not contains '%s' pattern", name, snapshotNameSeparator)
	}

	return nil
}

// validateCreationToken checks that the creation token of a new subvolume matches the requirements of a creation token.
func (d *NASBlockStorageDriver) validateCreationToken(name string) error {
	if !subvolumeCreationTokenRegex.MatchString(name) {
		return fmt.Errorf("subvolume internal name '%s' is not allowed; it must be 1-64 characters long, "+
			"begin with a letter, and contain only letters, digits, and hyphens", name)
	}
	return nil
}

// defaultCreateTimeout sets the driver timeout for volume create/delete operations.  Docker gets more time, since
// it doesn't have a mechanism to retry.
func (d *NASBlockStorageDriver) defaultCreateTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerCreateTimeout
	default:
		return api.VolumeCreateTimeout
	}
}

// defaultTimeout controls the driver timeout for most API operations.
func (d *NASBlockStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return api.DefaultTimeout
	}
}

// Initialize initializes this driver from the provided config.
func (d *NASBlockStorageDriver) Initialize(
	ctx context.Context, context tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "NASBlockStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

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

	// Should be called after ensuring a value for storage config is set
	d.helper = NewFileHelper(d.Config, context)

	if err = d.initializeAzureSDKClient(ctx, &d.Config); err != nil {
		return fmt.Errorf("error initializing %s SDK client; %v", d.Name(), err)
	}

	// Initialize the storage pool once Azure resources have been discovered
	if d.physicalPools, d.virtualPools, err = d.initializeStoragePools(ctx); err != nil {
		return fmt.Errorf("could not configure storage pools; %v", err)
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

	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = backendUUID
	d.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    d.Name(),
	}

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":              *config.StoragePrefix,
		"Size":                       config.Size,
		"ServiceLevel":               config.ServiceLevel,
		"NfsMountOptions":            config.NfsMountOptions,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	})
	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated).
func (d *NASBlockStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded.
func (d *NASBlockStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "NASBlockStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config.
func (d *NASBlockStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AzureNASStorageDriverConfig,
) {
	fields := LogFields{"Method": "populateConfigurationDefaults", "Type": "NASBlockStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> populateConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< populateConfigurationDefaults")

	if config.StoragePrefix == nil {
		defaultPrefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		defaultPrefix = strings.Replace(defaultPrefix, "_", "", -1)
		config.StoragePrefix = &defaultPrefix
	}

	if config.Size == "" {
		config.Size = defaultSubvolumeSizeStr
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}
	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":   *config.StoragePrefix,
		"Size":            config.Size,
		"ServiceLevel":    config.ServiceLevel,
		"NfsMountOptions": config.NfsMountOptions,
		"LimitVolumeSize": config.LimitVolumeSize,
	}).Debugf("Configuration defaults")

	return
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NASBlockStorageDriver) initializeStoragePools(
	ctx context.Context,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	physicalPools := make(map[string]storage.Pool)
	virtualPools := make(map[string]storage.Pool)

	// Need to identify the NFS protocol backend supports and make sure all of the filePoolVolumes follow the same
	// protocol
	nfsVersion, err := utils.GetNFSVersionFromMountOptions(d.Config.NfsMountOptions, "", supportedNFSVersions)
	if err != nil {
		return nil, nil, err
	}

	var protocolTypes string
	switch nfsVersion {
	case nfsVersion3:
		protocolTypes = api.ProtocolTypeNFSv3
	case nfsVersion4, nfsVersion41:
		protocolTypes = api.ProtocolTypeNFSv41
	}

	if len(d.Config.FilePoolVolumes) > 0 {
		filePoolVolumes, err := d.SDK.ValidateFilePoolVolumes(ctx, d.Config.FilePoolVolumes)
		if err != nil {
			return nil, nil, fmt.Errorf("error initializing physical pools: %v", err)
		}

		for _, filePoolVolume := range filePoolVolumes {
			name := fmt.Sprintf("%s_%s", filePoolVolume.Name, d.createFilePoolVolumePathHash(filePoolVolume))
			poolName := strings.Replace(name, "-", "", -1)

			if protocolTypes != "" && filePoolVolume.ProtocolTypes[0] != protocolTypes {
				Logc(ctx).Warnf("Protocol for filePoolVolume '%s' in pool '%s' is '%s' which does not match"+
					" NFSMountOptions's NFS version '%s'; thus NFSMountOptions version will be ignored",
					filePoolVolume.FullName, poolName, filePoolVolume.ProtocolTypes[0], protocolTypes)
			}

			pool := storage.NewStoragePool(nil, poolName)

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
			pool.InternalAttributes()[FilePoolVolumes] = filePoolVolume.FullName

			pool.SetSupportedTopologies(d.Config.SupportedTopologies)

			physicalPools[pool.Name()] = pool
		}
	}

	if len(d.Config.Storage) > 0 {
		Logc(ctx).Debug("One or more vpools defined.")

		// Report a pool for each virtual pool in the config
		for index, vpool := range d.Config.Storage {

			poolName := d.poolName(fmt.Sprintf("pool_%d", index))

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

			configFilePoolVolumes := d.Config.FilePoolVolumes
			if vpool.FilePoolVolumes != nil {
				configFilePoolVolumes = vpool.FilePoolVolumes
			}

			filePoolVolumes, err := d.SDK.ValidateFilePoolVolumes(ctx, configFilePoolVolumes)
			if err != nil {
				return nil, nil, fmt.Errorf("error initializing virtual pool '%s': %v", poolName, err)
			}

			for _, filePoolVolume := range filePoolVolumes {
				if protocolTypes != "" && filePoolVolume.ProtocolTypes[0] != protocolTypes {
					Logc(ctx).Warnf("Protocol for filePoolVolume '%s' in pool '%s' is '%s' which does not match"+
						" NFSMountOptions's NFS version '%s'; thus NFSMountOptions version will be ignored",
						filePoolVolume.FullName, poolName, filePoolVolume.ProtocolTypes[0], protocolTypes)
				}
			}

			pool := storage.NewStoragePool(nil, poolName)

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
			// TODO: When supporting multiple filePoolVolumes this will change
			pool.InternalAttributes()[FilePoolVolumes] = filePoolVolumes[0].FullName

			pool.SetSupportedTopologies(supportedTopologies)

			virtualPools[pool.Name()] = pool
		}
	}

	if len(physicalPools) == 0 && len(virtualPools) == 0 {
		return nil, nil, fmt.Errorf("filePoolVolumes is a required field")
	}

	return physicalPools, virtualPools, nil
}

// initializeAzureConfig parses the Azure config, mixing in the specified common config.
func (d *NASBlockStorageDriver) initializeAzureConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.AzureNASStorageDriverConfig, error) {
	fields := LogFields{"Method": "initializeAzureConfig", "Type": "NASBlockStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeAzureConfig")
	defer Logd(ctx, commonConfig.StorageDriverName, commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeAzureConfig")

	config := &drivers.AzureNASStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AzureNASStorageDriverConfig object
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration; %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		if err := config.InjectSecrets(backendSecret); err != nil {
			return nil, fmt.Errorf("could not inject backend secret; %v", err)
		}
	}

	return config, nil
}

// initializeAzureSDKClient creates and initializes an Azure SDK client.
func (d *NASBlockStorageDriver) initializeAzureSDKClient(
	ctx context.Context, config *drivers.AzureNASStorageDriverConfig,
) error {
	fields := LogFields{"Method": "initializeAzureSDKClient", "Type": "NASBlockStorageDriver"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> initializeAzureSDKClient")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< initializeAzureSDKClient")

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
		SubscriptionID:    config.SubscriptionID,
		TenantID:          config.TenantID,
		ClientID:          config.ClientID,
		ClientSecret:      config.ClientSecret,
		Location:          config.Location,
		StorageDriverName: config.StorageDriverName,
		DebugTraceFlags:   config.DebugTraceFlags,
		SDKTimeout:        sdkTimeout,
		MaxCacheAge:       maxCacheAge,
	})
	if err != nil {
		return err
	}

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.SDK == nil {
		d.SDK = client
	}

	// The storage pools are not required to be passed in for this driver
	return d.SDK.Init(ctx, nil)
}

// validate ensures the driver configuration and execution environment are valid and working.
func (d *NASBlockStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NASBlockStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	storagePrefix := *d.Config.StoragePrefix

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(storagePrefix); err != nil {
		return err
	}

	// Ensure length of the storage prefix is <=10
	if len(storagePrefix) > 10 {
		return fmt.Errorf("length of the storage prefix %s should be less than 11", *d.Config.StoragePrefix)
	}

	// Ensure storage prefix does not allow -- or ends with '-'
	if strings.Contains(storagePrefix, snapshotNameSeparator) {
		return fmt.Errorf("storage prefix '%s' contains '%s'", storagePrefix, snapshotNameSeparator)
	} else if storagePrefix[len(storagePrefix)-1:] == "-" {
		return fmt.Errorf("storage prefix '%s' ends with '-'", storagePrefix)
	}

	// Ensure user does not provide "ro" mount option
	if utils.AreMountOptionsInList(d.Config.NfsMountOptions, []string{"ro"}) {
		return fmt.Errorf("ReadOnly (ro) option is not supported in ANF subvolume backend nfsMountOptions; %s",
			d.Config.NfsMountOptions)
	}

	// Validate pool-level attributes
	allPools := make([]storage.Pool, 0, len(d.physicalPools)+len(d.virtualPools))

	for _, pool := range d.physicalPools {
		allPools = append(allPools, pool)
	}
	for _, pool := range d.virtualPools {
		allPools = append(allPools, pool)
	}

	// Validate pool-level Attributes
	for _, pool := range allPools {
		// Validate default size
		if _, err := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", pool.Name(), err)
		}
	}

	return nil
}

// Create a new subvolume.
func (d *NASBlockStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig,
	storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	creationToken := volConfig.InternalName
	fields := LogFields{
		"Method":        "Create",
		"Type":          "NASBlockStorageDriver",
		"creationToken": creationToken,
		"attrs":         volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Make sure we got a valid volume name
	if err := d.validateVolumeName(volConfig.Name); err != nil {
		return err
	}

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(creationToken); err != nil {
		return err
	}

	// If the subvolume already exists, bail out
	subvolumeExists, extantSubvolume, err := d.SDK.SubvolumeExists(ctx, volConfig, d.getAllFilePoolVolumes())
	if err != nil {
		return fmt.Errorf("error checking for existing subvolume %s; %v", creationToken, err)
	}

	if subvolumeExists {
		volConfig.InternalName = extantSubvolume.Name
		volConfig.InternalID = extantSubvolume.ID

		Logc(ctx).WithFields(LogFields{
			"name":  volConfig.InternalName,
			"state": extantSubvolume.ProvisioningState,
		}).Warning("Subvolume already exists.")

		// Get the reference object
		pollerKey := PollerKey{
			ID:        extantSubvolume.Name,
			Operation: "add",
		}

		poller := pollerResponseCache[pollerKey]

		// Wait for creation to complete
		if err = d.waitForSubvolumeCreate(ctx, extantSubvolume, poller); err != nil {
			return err
		}

		return drivers.NewVolumeExistsError(volConfig.InternalName)
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
		defaultSize, err := utils.ConvertSizeToBytes(storagePool.InternalAttributes()[Size])
		if err != nil {
			return fmt.Errorf("invalid size value '%s': %v", storagePool.InternalAttributes()[Size], err)
		}

		sizeBytes, err = strconv.ParseUint(defaultSize, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid size value '%s': %v", defaultSize, err)
		}
	}
	if checkMinVolumeSizeError := drivers.CheckMinVolumeSize(sizeBytes,
		MinimumSubvolumeSizeBytes); checkMinVolumeSizeError != nil {
		return checkMinVolumeSizeError
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"creationToken": creationToken,
		"size":          sizeBytes,
		"volume":        storagePool.InternalAttributes()[FilePoolVolumes],
	}).Debug("Creating subvolume.")

	subvolumeCreateRequest := &api.SubvolumeCreateRequest{
		CreationToken: creationToken,
		Volume:        storagePool.InternalAttributes()[FilePoolVolumes],
		Size:          int64(sizeBytes),
		Parent:        "", // Needed only when cloning
	}

	// Create the volume
	subvolume, poller, err := d.SDK.CreateSubvolume(ctx, subvolumeCreateRequest)
	if err != nil {
		return err
	}

	// Always save the ID so we can find the volume efficiently later
	volConfig.InternalID = subvolume.ID

	// Save the Poller's reference for later uses (if needed)
	pollerKey := PollerKey{
		ID:        subvolume.Name,
		Operation: "add",
	}

	pollerResponseCache[pollerKey] = poller

	// Wait for creation to complete
	return d.waitForSubvolumeCreate(ctx, subvolume, poller)
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NASBlockStorageDriver) CreateClone(
	ctx context.Context, sourceVolConfig, volConfig *storage.VolumeConfig, _ storage.Pool,
) error {
	creationToken := volConfig.InternalName
	source := volConfig.CloneSourceVolume
	snapshot := volConfig.CloneSourceSnapshot
	isFromSnapshot := snapshot != ""

	fields := LogFields{
		"Method":        "CreateClone",
		"Type":          "NASBlockStorageDriver",
		"creationToken": creationToken,
		"source":        source,
		"snapshot":      snapshot,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	// Make sure we got a valid name
	if err := d.validateVolumeName(volConfig.Name); err != nil {
		return err
	}

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(creationToken); err != nil {
		return err
	}

	sourceInternalID := sourceVolConfig.InternalID

	// Check if called from CreateClone and is from a snapshot
	if isFromSnapshot {
		snapshotInternalName := d.helper.GetSnapshotInternalName(source, snapshot)

		subscription, resourceGroup, _, netappAccount, cPoolName, volumeName, _, err := api.ParseSubvolumeID(sourceVolConfig.InternalID)
		if err != nil {
			return fmt.Errorf("failed to parse source volume ID %s name: %v", sourceVolConfig.InternalID, err)
		}

		// ID of the snapshot subvolume should have all the same attributes as the source volume except the name
		sourceInternalID = api.CreateSubvolumeID(subscription, resourceGroup, netappAccount, cPoolName, volumeName,
			snapshotInternalName)
	}

	// Get the source subvolume
	sourceSubvolume, err := d.SDK.SubvolumeByID(ctx, sourceInternalID, false)
	if err != nil {
		return fmt.Errorf("could not find source volume; %v", err)
	}

	// If the specified subvolume already exists, return an error
	subvolumeExists, extantSubvolume, err := d.SDK.SubvolumeExists(ctx, volConfig, d.getAllFilePoolVolumes())
	if err != nil {
		return fmt.Errorf("error checking for existing subvolume %s; %v", creationToken, err)
	}
	if subvolumeExists {
		volConfig.InternalName = extantSubvolume.Name
		volConfig.InternalID = extantSubvolume.ID

		Logc(ctx).WithFields(LogFields{
			"name":  volConfig.InternalName,
			"state": extantSubvolume.ProvisioningState,
		}).Warning("Subvolume already exists.")

		// Get the reference object
		pollerKey := PollerKey{
			ID:        extantSubvolume.Name,
			Operation: "add",
		}

		poller := pollerResponseCache[pollerKey]

		// Wait for creation to complete
		if err = d.waitForSubvolumeCreate(ctx, extantSubvolume, poller); err != nil {
			return err
		}

		return drivers.NewVolumeExistsError(volConfig.InternalName)
	}

	filePoolVolume := api.CreateVolumeFullName(sourceSubvolume.ResourceGroup, sourceSubvolume.NetAppAccount,
		sourceSubvolume.CapacityPool, sourceSubvolume.Volume)

	Logc(ctx).WithFields(LogFields{
		"creationToken": creationToken,
		"size":          sourceSubvolume.Size, // This may come out to be zero and has no affect on clone size
		"volume":        filePoolVolume,
		"parentPath":    sourceSubvolume.Name,
	}).Debug("Creating subvolume clone.")

	// Create the clone based on given file
	subvolumeCreateRequest := &api.SubvolumeCreateRequest{
		CreationToken: creationToken,
		Volume:        filePoolVolume,
		Size:          sourceSubvolume.Size,
		Parent:        sourceSubvolume.Name, // Needed only when cloning
	}
	// Create the volume
	subvolume, poller, err := d.SDK.CreateSubvolume(ctx, subvolumeCreateRequest)
	if err != nil {
		return err
	}

	// Always save the ID so we can find the volume efficiently later
	volConfig.InternalID = subvolume.ID

	// Save the Poller's reference for later uses (if needed)
	pollerKey := PollerKey{
		ID:        subvolume.Name,
		Operation: "add",
	}

	pollerResponseCache[pollerKey] = poller

	// Wait for creation to complete
	return d.waitForSubvolumeCreate(ctx, subvolume, poller)
}

// Import finds an existing subvolume and makes it available for containers. If ImportNotManaged is false, the
// subvolume is fully brought under Trident's management.
func (d *NASBlockStorageDriver) Import(
	ctx context.Context, volConfig *storage.VolumeConfig, originalName string,
) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "NASBlockStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Make sure the original name is in an acceptable format
	if err := d.validateCreationToken(originalName); err != nil {
		return err
	}

	// Ensure subvolume is actually a subvolume, and it isn't a "snapshot" subvolume
	if !d.isFileValidVolume(ctx, originalName) {
		return fmt.Errorf("ineligible for import; subvolume %s is a snapshot subvolume", originalName)
	}

	subvolumeWithMetadata, err := d.SDK.SubvolumeByCreationToken(ctx, originalName, d.getAllFilePoolVolumes(), true)
	if err != nil {
		return fmt.Errorf("could not find subvolume %s; %v", originalName, err)
	}

	if checkMinVolumeSizeError := drivers.CheckMinVolumeSize(uint64(subvolumeWithMetadata.Size),
		MinimumSubvolumeSizeBytes); checkMinVolumeSizeError != nil {
		return fmt.Errorf("size error; %v", checkMinVolumeSizeError)
	}

	volConfig.Size = strconv.FormatInt(subvolumeWithMetadata.Size, 10)

	// The ANF subvolume creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = originalName

	// Always save the ID so we can find the volume efficiently later
	volConfig.InternalID = subvolumeWithMetadata.ID

	return nil
}

func (d *NASBlockStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "NASBlockStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	// Rename is only needed for the import workflow, and we aren't currently renaming the
	// ANF subvolume when importing, so do nothing here lest we set the subvolume name incorrectly
	// during an import failure cleanup.
	return nil
}

// waitForSubvolumeCreate waits for volume creation to complete by reaching the Available state.  If the
// volume reaches a terminal state (Error), the volume is deleted.  If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
func (d *NASBlockStorageDriver) waitForSubvolumeCreate(ctx context.Context, subvolume *api.Subvolume,
	poller api.PollerResponse,
) error {
	var pollForError bool

	state, err := d.SDK.WaitForSubvolumeState(
		ctx, subvolume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		logFields := LogFields{"subvolume": subvolume}

		switch state {

		case api.StateAccepted, api.StateCreating:
			Logc(ctx).WithFields(logFields).Debugf("Subvolume is in %s state.", state)
			return utils.VolumeCreatingError(err.Error())

		case api.StateDeleting:
			// Wait for deletion to complete
			_, errDelete := d.SDK.WaitForSubvolumeState(
				ctx, subvolume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Subvolume could not be cleaned up and must be manually deleted.")
			}

		case api.StateError:
			// Delete a failed volume
			_, errDelete := d.SDK.DeleteSubvolume(ctx, subvolume)
			if errDelete != nil {
				Logc(ctx).WithFields(logFields).WithError(errDelete).Error(
					"Subvolume could not be cleaned up and must be manually deleted.")
			} else {
				Logc(ctx).WithField("subvolume", subvolume.Name).Info("Subvolume deleted.")
			}

			pollForError = true

		case api.StateMoving:
			fallthrough

		default:
			Logc(ctx).WithFields(logFields).Errorf("unexpected subvolume state %s found for subvolume", state)
			pollForError = true
		}
	}

	// If here, it mean volume might be successful, or in deleting, error, moving or unexpected state,
	// and not in creating state, so it should be safe to remove it from futures cache
	pollerKey := PollerKey{
		ID:        subvolume.Name,
		Operation: "add",
	}

	delete(pollerResponseCache, pollerKey)

	if pollForError && poller != nil {
		if err != nil && state == api.StateError {
			Logc(ctx).Errorf("failed to create subvolume: %v", poller.Result(ctx))
		} else {
			return poller.Result(ctx)
		}
	}

	return nil
}

// Destroy deletes a volume.
func (d *NASBlockStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	var extantSubvolume *api.Subvolume
	var subvolumeExists bool
	var err error

	creationToken := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NASBlockStorageDriver",
		"name":   creationToken,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// In case where subvolume creation fails it may not contain an internalID, so clean it up using creation token
	if volConfig.InternalID == "" {
		subvolumeExists, extantSubvolume, err = d.SDK.SubvolumeExists(ctx, volConfig, d.getAllFilePoolVolumes())
		if err != nil {
			return fmt.Errorf("error checking for existing subvolume %s; %v", creationToken, err)
		}

		if !subvolumeExists {
			Logc(ctx).WithField("subvolume", creationToken).Warn("Subvolume already deleted.")
			return nil
		} else if extantSubvolume.ProvisioningState == api.StateDeleting {
			// This is a retry, so give it more time before giving up again.
			_, err = d.SDK.WaitForSubvolumeState(
				ctx, extantSubvolume, api.StateDeleted, []string{api.StateError}, d.volumeCreateTimeout)
			return err
		}
	} else {
		_, resourceGroup, _, netappAccount, cPoolName, volumeName, _, err := api.ParseSubvolumeID(volConfig.InternalID)
		if err != nil {
			return fmt.Errorf("error parsing volume config internal ID '%s': %v", volConfig.InternalName, err)
		}

		extantSubvolume = &api.Subvolume{
			ID:            volConfig.InternalID,
			ResourceGroup: resourceGroup,
			NetAppAccount: netappAccount,
			CapacityPool:  cPoolName,
			Volume:        volumeName,
			Name:          creationToken,
		}
	}

	// Delete the subvolume
	poller, err := d.SDK.DeleteSubvolume(ctx, extantSubvolume)
	if err != nil {
		if !utils.IsNotFoundError(err) {
			return fmt.Errorf("error deleting subvolume %s; %v", creationToken, err)
		}
	}

	Logc(ctx).WithField("subvolume", extantSubvolume.Name).Info("subvolume deleted.")

	// Wait for deletion to complete
	state, err := d.SDK.WaitForSubvolumeState(ctx, extantSubvolume, api.StateDeleted, []string{api.StateError},
		d.defaultTimeout())

	if err != nil && state == api.StateError {
		Logc(ctx).WithField("subvolume", extantSubvolume.Name).Errorf("failed to delete volume: %v", poller.Result(ctx))
	}

	return err
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASBlockStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	// Get the subvolume
	creationToken := volConfig.InternalName

	fields := LogFields{
		"Method": "Publish",
		"Type":   "NASBlockStorageDriver",
		"name":   volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Get the subvolume's parent ANF volume
	volume, err := d.SDK.SubvolumeParentVolume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find subvolume's ('%s') parent volume: %v", creationToken, err)
	}

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volume.Name)
	}

	// Set the correct NFS mount option based on volume's protocol
	NFSMountOption := fmt.Sprintf("vers=%s", strings.TrimPrefix(volume.ProtocolTypes[0], api.ProtocolTypeNFSPrefix))
	mountOptions := utils.SetNFSVersionMountOptions(d.Config.NfsMountOptions, NFSMountOption)

	// Subvolume mount options can only be specified via tha storage class.
	subvolumeMountOptions := ""
	if volConfig.MountOptions != "" {
		subvolumeMountOptions = volConfig.MountOptions
	}

	// Get the fstype
	fsType := drivers.DefaultFileSystemType
	if volConfig.FileSystem != "" {
		fsType = volConfig.FileSystem
	}

	// xfs volumes are always mounted with '-o nouuid' to allow clones to be mounted to the same node as the source
	if strings.Contains(fsType, tridentconfig.FsXfs) {
		subvolumeMountOptions = drivers.EnsureMountOption(subvolumeMountOptions, drivers.MountOptionNoUUID)
	}

	// Just use the first mount target found
	publishInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
	publishInfo.NfsPath = "/" + volume.CreationToken
	publishInfo.NfsUniqueID = d.createFilePoolVolumePathHash(volume)
	publishInfo.SubvolumeName = volConfig.InternalName
	publishInfo.MountOptions = strings.TrimPrefix(mountOptions, "-o ")
	publishInfo.SubvolumeMountOptions = strings.TrimPrefix(subvolumeMountOptions, "-o ")
	publishInfo.FilesystemType = fsType

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASBlockStorageDriver) CanSnapshot(
	_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	return nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NASBlockStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	snapName := snapConfig.Name
	internalVolName := snapConfig.VolumeInternalName
	externalVolName := snapConfig.VolumeName

	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NASBlockStorageDriver",
		"snapshotName": snapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	// Get the source subvolume
	sourceSubvolumeExists, sourceSubvolume, err := d.SDK.SubvolumeExistsByID(ctx, volConfig.InternalID)
	if err != nil {
		return nil, fmt.Errorf("could not find source subvolume '%s'; %v", volConfig.InternalID, err)
	}
	if !sourceSubvolumeExists {
		return nil, fmt.Errorf("source subvolume '%s' does not exist", volConfig.Name)
	}

	// Create the snapshot name/string
	creationToken := d.helper.GetSnapshotInternalName(externalVolName, snapName)

	// Based on volume's internal name, snapshot ID can be identified
	snapshotInternalID := api.CreateSubvolumeID(d.Config.SubscriptionID, sourceSubvolume.ResourceGroup,
		sourceSubvolume.NetAppAccount, sourceSubvolume.CapacityPool, sourceSubvolume.Volume, creationToken)

	snapshotExists, extantSubvolume, err := d.SDK.SubvolumeExistsByID(ctx, snapshotInternalID)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing snapshot %s; %v", snapshotInternalID, err)
	}
	if !snapshotExists {
		return nil, nil
	}

	if extantSubvolume.ProvisioningState != api.StateAvailable {
		return nil, fmt.Errorf("snapshot %s state is %s", creationToken, extantSubvolume.ProvisioningState)
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName":         snapName,
		"snapshotInternalName": creationToken,
		"volumeName":           internalVolName,
	}).Debug("Found snapshot.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   time.Time{}.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// GetSnapshots returns the list of snapshots associated with the specified subvolume
func (d *NASBlockStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName
	externalVolName := volConfig.Name
	prefix := *d.Config.StoragePrefix

	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NASBlockStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	// Get the source subvolume
	sourceSubvolume, err := d.SDK.Subvolume(ctx, volConfig, false)
	if err != nil {
		return nil, fmt.Errorf("could not find source subvolume '%s'; %v", volConfig.InternalID, err)
	}

	// Fetch list of all the subvolumes from parent volume of the above volConfig
	subvolumes, err := d.SDK.Subvolumes(ctx, []string{
		api.CreateVolumeFullName(sourceSubvolume.ResourceGroup,
			sourceSubvolume.NetAppAccount, sourceSubvolume.CapacityPool, sourceSubvolume.Volume),
	})
	if err != nil {
		return nil, err
	}

	snapshots := make([]*storage.Snapshot, 0)

	for _, subvolume := range *subvolumes {

		// Filter out subvolume without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(subvolume.Name, prefix) {
			continue
		}

		if !d.helper.IsValidSnapshotInternalName(subvolume.Name) {
			continue
		}

		// TODO: Filter out subvolumes whose parentPath does not match parent subvolume's internal name.
		//       Unfortunately SDK does not return this information, thus using below alternative.
		if d.helper.GetSnapshotSuffixFromSnapshotInternalName(subvolume.Name) != d.helper.GetSnapshotSuffix(externalVolName) {
			continue
		}

		snapName := d.helper.GetSnapshotNameFromSnapInternalName(subvolume.Name)
		snapshot := &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snapName,
				InternalName:       subvolume.Name,
				VolumeName:         externalVolName,
				VolumeInternalName: internalVolName,
			},
			Created:   time.Time{}.UTC().Format(storage.SnapshotTimestampFormat),
			SizeBytes: 0,
			State:     storage.SnapshotStateOnline,
		}
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume
// NOTE: In ANF Subvolumes there is no concept of snapshots, therefore any new snapshot is another
// subvolume copy of the source subvolume.
func (d *NASBlockStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	snapName := snapConfig.Name
	internalVolName := snapConfig.VolumeInternalName

	var poller api.PollerResponse

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NASBlockStorageDriver",
		"snapshotName": snapName,
		"volumeName":   internalVolName,
	}

	if d.Config.DebugTraceFlags["method"] {
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	// Validate snapshot name
	if err := d.validateSnapshotName(snapName); err != nil {
		return nil, err
	}

	// Create the snapshot name/string
	creationToken := d.helper.GetSnapshotInternalName(snapConfig.VolumeName, snapName)

	// Make sure we got a valid creation token
	if err := d.validateCreationToken(creationToken); err != nil {
		return nil, err
	}

	_, resourceGroup, _, netappAccount, cPoolName, volumeName, sourceSubvolumeName,
		err := api.ParseSubvolumeID(volConfig.InternalID)
	if err != nil {
		return nil, fmt.Errorf("error parsing source volume config internal ID '%s': %v", volConfig.InternalName, err)
	}

	// Based on volume's internal ID, snapshot ID can be identified
	snapshotInternalID := api.CreateSubvolumeID(d.Config.SubscriptionID, resourceGroup,
		netappAccount, cPoolName, volumeName, creationToken)

	// Check if the specified snapshot subvolume already exists
	snapshotExists, subvolume, err := d.SDK.SubvolumeExistsByID(ctx, snapshotInternalID)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing snapshot %s; %v", creationToken, err)
	}

	if !snapshotExists {
		// NOTE: Do not get the source subvolume, that later causes get metadata to fail.

		// Create name of the volume where this snapshot subvolume will live
		filePoolVolume := api.CreateVolumeFullName(resourceGroup, netappAccount, cPoolName, volumeName)

		Logc(ctx).WithFields(LogFields{
			"creationToken": creationToken,
			"volume":        filePoolVolume,
			"parentPath":    internalVolName,
		}).Debug("Creating subvolume snapshot.")

		// Create the snapshot (a subvolume) based on given file
		subvolumeCreateRequest := &api.SubvolumeCreateRequest{
			CreationToken: creationToken,
			Volume:        filePoolVolume,
			Parent:        sourceSubvolumeName, // Needed only when cloning
		}

		// Create the snapshot
		subvolume, poller, err = d.SDK.CreateSubvolume(ctx, subvolumeCreateRequest)
		if err != nil {
			return nil, err
		}
	}

	// Reading the creation timestamp from the subvolume metadata is expensive, instead
	// use the current timestamp
	createdAt := time.Now()

	// Save the Poller's reference for later uses (if needed)
	pollerKey := PollerKey{
		ID:        subvolume.Name,
		Operation: "add",
	}

	pollerResponseCache[pollerKey] = poller

	if err = d.waitForSubvolumeCreate(ctx, subvolume, poller); err != nil {
		return nil, err
	}

	// For this driver the internal name and the name are different so set the internal name
	snapConfig.InternalName = creationToken

	Logc(ctx).WithFields(LogFields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Info("Snapshot created.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   createdAt.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASBlockStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	snapName := snapConfig.Name
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NASBlockStorageDriver",
		"snapshotName": snapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return utils.UnsupportedError(fmt.Sprintf("restoring snapshots is not supported by backend type %s", d.Name()))
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASBlockStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) error {
	snapName := snapConfig.Name
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NASBlockStorageDriver",
		"snapshotName": snapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	creationToken := d.helper.GetSnapshotInternalName(snapConfig.VolumeName, snapName)

	subscriptionID, resourceGroup, _, netappAccount, cPoolName, volumeName, _,
		err := api.ParseSubvolumeID(volConfig.InternalID)
	if err != nil {
		return fmt.Errorf("error parsing source volume config internal ID '%s': %v", volConfig.InternalName, err)
	}

	subvolumeID := api.CreateSubvolumeID(subscriptionID, resourceGroup, netappAccount, cPoolName, volumeName,
		creationToken)

	subvolume := &api.Subvolume{
		ID:            subvolumeID,
		ResourceGroup: resourceGroup,
		NetAppAccount: netappAccount,
		CapacityPool:  cPoolName,
		Volume:        volumeName,
		Name:          creationToken,
	}

	// If the specified snapshot subvolume already exists, return an error
	poller, err := d.SDK.DeleteSubvolume(ctx, subvolume)
	if err != nil {
		if !utils.IsNotFoundError(err) {
			return fmt.Errorf("error deleting snapshot %s; %v", creationToken, err)
		}
	}

	Logc(ctx).Debugf("Snapshot %s deleted.", creationToken)

	// Wait for deletion to complete
	state, err := d.SDK.WaitForSubvolumeState(ctx, subvolume, api.StateDeleted, []string{api.StateError},
		d.defaultTimeout())

	if err != nil && state == api.StateError {
		Logc(ctx).WithField("subvolume", subvolume.Name).Errorf("failed to delete volume: %v", poller.Result(ctx))
	}

	return err
}

// Get tests for the existence of a volume
func (d *NASBlockStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASBlockStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	if _, err := d.SDK.SubvolumeByCreationToken(ctx, name, d.getAllFilePoolVolumes(), false); err != nil {
		return fmt.Errorf("could not get volume %s; %v", name, err)
	}

	return nil
}

// Resize increases a volume's quota.
func (d *NASBlockStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method":    "Resize",
		"Type":      "NASBlockStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Get the subvolume
	subvolumeWithMetadata, err := d.SDK.Subvolume(ctx, volConfig, true)
	if err != nil {
		return fmt.Errorf("could not find subvolume %s; %v", name, err)
	}

	if subvolumeWithMetadata.ProvisioningState != api.StateAvailable {
		return fmt.Errorf("subvolume %s state is %s, not available", name, subvolumeWithMetadata.ProvisioningState)
	}

	volConfig.Size = strconv.FormatUint(uint64(subvolumeWithMetadata.Size), 10)

	// If the subvolume is already the requested size, there's nothing to do
	if int64(sizeBytes) == subvolumeWithMetadata.Size {
		return nil
	}

	// Make sure we're not shrinking the volume
	if int64(sizeBytes) < subvolumeWithMetadata.Size {
		return fmt.Errorf("requested size %d is less than existing subvolume size %d", sizeBytes,
			subvolumeWithMetadata.Size)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	if _, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Resize the subvolume
	if err = d.SDK.ResizeSubvolume(ctx, subvolumeWithMetadata, int64(sizeBytes)); err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

// GetStorageBackendSpecs retrieves storage capabilities and register pools with specified backend.
func (d *NASBlockStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	backend.SetName(d.BackendName())

	virtualPoolsExist := len(d.virtualPools) > 0

	for _, pool := range d.physicalPools {
		pool.SetBackend(backend)
		if !virtualPoolsExist {
			backend.AddStoragePool(pool)
		}
	}

	for _, pool := range d.virtualPools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	return nil
}

// CreatePrepare is called prior to volume creation. Currently its only role is to create the internal volume name.
func (d *NASBlockStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASBlockStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	physicalPoolNames := make([]string, 0)
	for poolName := range d.physicalPools {
		physicalPoolNames = append(physicalPoolNames, poolName)
	}
	return physicalPoolNames
}

// GetInternalVolumeName accepts the name of a volume being created and returns what the internal name
// should be, depending on backend requirements and Trident's operating context.
func (d *NASBlockStorageDriver) GetInternalVolumeName(ctx context.Context, name string) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// With an external store, any transformation of the name is fine
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = internal + api.SubvolumeNameSeparator + "0"
		Logc(ctx).WithField("volumeInternal", internal).Debug("Modified volume name for internal name.")
		return internal
	}
}

func (d *NASBlockStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	creationToken := volConfig.InternalName

	fields := LogFields{
		"Method": "CreateFollowup",
		"Type":   "NASBlockStorageDriver",
		"name":   creationToken,
	}

	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

	subvolume, err := d.SDK.Subvolume(ctx, volConfig, false)
	if err != nil {
		return fmt.Errorf("could not find subvolume %s; %v", creationToken, err)
	}

	volume, err := d.SDK.SubvolumeParentVolume(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("could not find subvolume's ('%s') parent volume: %v", creationToken, err)
	}

	// Ensure subvolume is in a good state
	if subvolume.ProvisioningState == api.StateError {
		return fmt.Errorf("subvolume %s is in %s state", creationToken, subvolume.ProvisioningState)
	}

	// Set the correct NFS mount option based on volume's protocol
	NFSMountOption := fmt.Sprintf("vers=%s", strings.TrimPrefix(volume.ProtocolTypes[0], api.ProtocolTypeNFSPrefix))
	mountOptions := utils.SetNFSVersionMountOptions(d.Config.NfsMountOptions, NFSMountOption)

	if len(volume.MountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volume.Name)
	}

	// Just use the first mount target found
	volConfig.AccessInfo.NfsServerIP = (volume.MountTargets)[0].IPAddress
	volConfig.AccessInfo.NfsPath = "/" + volume.CreationToken
	volConfig.AccessInfo.NfsUniqueID = d.createFilePoolVolumePathHash(volume)
	volConfig.AccessInfo.SubvolumeName = volConfig.InternalName
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(mountOptions, "-o ")

	if !strings.Contains(volConfig.FileSystem, "nfs/") {
		volConfig.FileSystem = fmt.Sprintf("nfs/%s", volConfig.FileSystem)
	}

	return nil
}

// GetProtocol returns the protocol supported by this driver (BlockOnFile).
func (d *NASBlockStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.BlockOnFile
}

// StoreConfig adds this backend's config to the persistent config struct, as needed by Trident's persistence layer.
func (d *NASBlockStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AzureConfig = &d.Config
}

// GetExternalConfig returns a clone of this backend's config, sanitized for external consumption.
func (d *NASBlockStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AzureNASStorageDriverConfig
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
func (d *NASBlockStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {
	subvolumeWithMetadata, err := d.SDK.SubvolumeByCreationToken(ctx, name, d.getAllFilePoolVolumes(), true)
	if err != nil {
		return nil, fmt.Errorf("could not find subvolume %s: %v", name, err)
	}

	return d.getSubvolumeExternal(subvolumeWithMetadata), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASBlockStorageDriver) GetVolumeExternalWrappers(
	ctx context.Context, channel chan *storage.VolumeExternalWrapper,
) {
	fields := LogFields{"Method": "GetVolumeExternalWrappers", "Type": "NASBlockStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetVolumeExternalWrappers")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetVolumeExternalWrappers")

	// Let the caller know we're done by closing the channel
	defer close(channel)

	prefix := *d.Config.StoragePrefix

	subvolumes, err := d.SDK.Subvolumes(ctx, d.getAllFilePoolVolumes())
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	for _, subvolume := range *subvolumes {

		// Filter out subvolume in an unavailable state
		switch subvolume.ProvisioningState {
		case api.StateDeleting, api.StateDeleted, api.StateError:
			continue
		}

		// Filter out subvolume without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(subvolume.Name, prefix) {
			continue
		}

		if !d.isFileValidVolume(ctx, subvolume.Name) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getSubvolumeExternal(subvolume), Error: nil}
	}
}

func (d *NASBlockStorageDriver) isFileValidVolume(ctx context.Context, subvolumeName string) bool {
	// Skip over files which are "snapshots" of other files
	if d.helper.GetSnapshotNameFromSnapInternalName(subvolumeName) != "" {
		Logc(ctx).WithField("path", subvolumeName).Debug("Skipping file snapshot.")
		return false
	}

	return true
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASBlockStorageDriver) getSubvolumeExternal(subVolumeAttrs *api.Subvolume) *storage.VolumeExternal {
	internalName := subVolumeAttrs.Name
	name := internalName

	// Remove Prefix
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix+"-"):]
	}

	// Remove Suffix
	name = strings.Split(name, api.SubvolumeNameSeparator)[0]

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(subVolumeAttrs.Size, 10),
		Protocol:        tridentconfig.BlockOnFile,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
		ServiceLevel:    "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   subVolumeAttrs.Volume,
	}
}

// Implement stringer interface for the NASBlockStorageDriver driver.
func (d NASBlockStorageDriver) String() string {
	return utils.ToStringRedacted(&d, []string{"SDK"}, d.GetExternalConfig(context.Background()))
}

// GoString implements GoStringer interface for the NASBlockStorageDriver driver.
func (d NASBlockStorageDriver) GoString() string {
	return d.String()
}

// GetUpdateType returns a bitmap populated with updates to the driver.
func (d *NASBlockStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASBlockStorageDriver)
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
func (d *NASBlockStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, _ string) error {
	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NASBlockStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d NASBlockStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d *NASBlockStorageDriver) getAllFilePoolVolumes() []string {
	virtual := len(d.virtualPools) > 0

	var candidateFileVolumePools []string

	if virtual {
		for _, vpool := range d.virtualPools {
			if !utils.SliceContainsString(candidateFileVolumePools, vpool.InternalAttributes()[FilePoolVolumes]) {
				candidateFileVolumePools = append(candidateFileVolumePools, vpool.InternalAttributes()[FilePoolVolumes])
			}
		}
	} else {
		candidateFileVolumePools = d.Config.FilePoolVolumes
	}

	return candidateFileVolumePools
}

func (d *NASBlockStorageDriver) createFilePoolVolumePathHash(filePoolVolume *api.FileSystem) string {
	// volume path for hash: subscriptionID/resourceGroup/netappAccount/capacityPool/volume
	// This volume path is unique to a filePoolVolume across subscriptions
	volumePath := fmt.Sprintf("%s/%s/%s/%s/%s", d.Config.SubscriptionID, filePoolVolume.ResourceGroup,
		filePoolVolume.NetAppAccount, filePoolVolume.CapacityPool, filePoolVolume.Name)
	sha256Hash := sha256.Sum256([]byte(volumePath))

	return fmt.Sprintf("%032x", sha256Hash[:RequiredHashLength])
}
