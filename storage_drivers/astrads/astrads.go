// Copyright 2022 NetApp, Inc. All Rights Reserved.

package astrads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/mitchellh/copystructure"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/astrads/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes = uint64(20971520) // 20 MB
	labelPrefix            = "trident.netapp.io/"

	minimumAstraDSVersion  = "2021.10.0"
	minimumSnapshotReserve = 0
	maximumSnapshotReserve = 90

	defaultExportPolicy    = "default"
	defaultUnixPermissions = "0777"
	defaultSnapshotReserve = "5"
	defaultSnapshotDir     = "false"
	defaultNfsMountOptions = "vers=4.1"
	defaultLimitVolumeSize = ""

	// Constants for internal pool attributes

	Size            = "size"
	ExportPolicy    = "exportPolicy"
	UnixPermissions = "unixPermissions"
	SnapshotReserve = "snapshotReserve"
	SnapshotDir     = "snapshotDir"
	QosPolicy       = "qosPolicy"
	Region          = "region"
	Zone            = "zone"

	volumeCreateTimeout   = 10 * time.Second
	snapshotCreateTimeout = 30 * time.Second
	volumeResizeTimeout   = 10 * time.Second
	defaultDeleteTimeout  = 10 * time.Second
	maxAnnotationLength   = 256 * 1024
)

// StorageDriver is for storage provisioning using Cloud Volumes Service in GCP
type StorageDriver struct {
	initialized         bool
	Config              drivers.AstraDSStorageDriverConfig
	API                 *api.Clients
	randomID            string
	nameRegexp          *regexp.Regexp
	csiRegexp           *regexp.Regexp
	apiRegions          []string
	pools               map[string]storage.Pool
	volumeCreateTimeout time.Duration
}

// Name returns the name of this driver
func (d *StorageDriver) Name() string {
	return drivers.AstraDSStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *StorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		return d.Config.Cluster
	}
}

// poolName constructs the name of the pool reported by this driver instance
func (d *StorageDriver) poolName(name string) string {
	return fmt.Sprintf("%s_%s", d.BackendName(), strings.Replace(name, "-", "", -1))
}

// validateName checks that the name of a new volume matches the requirements of a Kubernetes object name
func (d *StorageDriver) validateName(name string) error {
	if !d.nameRegexp.MatchString(name) {
		return fmt.Errorf("volume name %s is not allowed; a DNS-1123 name must consist of lower case alphanumeric "+
			"characters, '-' or '.', and must start and end with an alphanumeric character", name)
	}
	return nil
}

// defaultCreateTimeout sets the driver timeout for volume create/delete operations.  Docker gets more time, since
// it doesn't have a mechanism to retry.
func (d *StorageDriver) defaultCreateTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerCreateTimeout
	default:
		return volumeCreateTimeout
	}
}

// defaultDeleteTimeout controls the driver timeout for most API operations.
func (d *StorageDriver) defaultDeleteTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return defaultDeleteTimeout
	}
}

// Initialize initializes this driver from the provided config
func (d *StorageDriver) Initialize(
	ctx context.Context, context tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, _ string,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	commonConfig.DriverContext = context
	d.nameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`) // RFC 1123
	d.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// Parse the config
	config, err := d.initializeAstraDSConfig(ctx, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
	}
	d.Config = *config

	if err = d.populateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults; %v", err)
	}

	// Validate config fields needed to create API client
	if d.Config.Kubeconfig == "" {
		return errors.New("kubeconfig not specified in backend config")
	}
	if d.Config.Cluster == "" {
		return errors.New("cluster not specified in backend config")
	}
	if d.Config.Namespace == "" {
		return errors.New("namespace not specified in backend config")
	}

	kubeConfigBytes, err := base64.StdEncoding.DecodeString(d.Config.Kubeconfig)
	if err != nil {
		return fmt.Errorf("kubeconfig is not base64 encoded: %v", err)
	}

	if d.API, err = api.NewClient(ctx, kubeConfigBytes, d.Config.Namespace); err != nil {
		return fmt.Errorf("error initializing %s API client; %v", d.Name(), err)
	}
	if err = d.API.DiscoverCluster(ctx, d.Config.Cluster); err != nil {
		return fmt.Errorf("error discovering %s cluster; %v", d.Config.Cluster, err)
	}
	d.Config.ClusterUUID = d.API.Cluster.UUID
	d.Config.KubeSystemUUID = d.API.KubeSystemUUID

	if err = d.initializeStoragePools(ctx); err != nil {
		return fmt.Errorf("could not configure storage pools; %v", err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver; %v", d.Name(), err)
	}

	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated)
func (d *StorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded
func (d *StorageDriver) Terminate(ctx context.Context, _ string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *StorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AstraDSStorageDriverConfig,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "populateConfigurationDefaults", "Type": "StorageDriver"}
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

	if config.ExportPolicy == "" {
		config.ExportPolicy = defaultExportPolicy
	}

	if config.UnixPermissions == "" {
		config.UnixPermissions = defaultUnixPermissions
	}

	if config.SnapshotReserve == "" {
		config.SnapshotReserve = defaultSnapshotReserve
	}

	if config.SnapshotDir == "" {
		config.SnapshotDir = defaultSnapshotDir
	}

	if config.NfsMountOptions == "" {
		config.NfsMountOptions = defaultNfsMountOptions
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	d.volumeCreateTimeout = d.defaultCreateTimeout()

	Logc(ctx).WithFields(log.Fields{
		"StoragePrefix":   *config.StoragePrefix,
		"RequestedSize":   config.Size,
		"ExportPolicy":    config.ExportPolicy,
		"UnixPermissions": config.UnixPermissions,
		"SnapshotReserve": config.SnapshotReserve,
		"SnapshotDir":     config.SnapshotDir,
		"NfsMountOptions": config.NfsMountOptions,
		"LimitVolumeSize": config.LimitVolumeSize,
	}).Debugf("Configuration defaults")

	return nil
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *StorageDriver) initializeStoragePools(ctx context.Context) error {

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
		pool.Attributes()[sa.ProvisioningType] = sa.NewStringOffer("thin")
		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)
		if d.Config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes()[Size] = d.Config.Size
		pool.InternalAttributes()[ExportPolicy] = d.Config.ExportPolicy
		pool.InternalAttributes()[UnixPermissions] = d.Config.UnixPermissions
		pool.InternalAttributes()[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes()[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes()[QosPolicy] = d.Config.QosPolicy
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

			exportPolicy := d.Config.ExportPolicy
			if vpool.ExportPolicy != "" {
				exportPolicy = vpool.ExportPolicy
			}

			unixPermissions := d.Config.UnixPermissions
			if vpool.UnixPermissions != "" {
				unixPermissions = vpool.UnixPermissions
			}

			snapshotReserve := d.Config.SnapshotReserve
			if vpool.SnapshotReserve != "" {
				snapshotReserve = vpool.SnapshotReserve
			}

			snapshotDir := d.Config.SnapshotDir
			if vpool.SnapshotDir != "" {
				snapshotDir = vpool.SnapshotDir
			}

			supportedTopologies := d.Config.SupportedTopologies
			if vpool.SupportedTopologies != nil {
				supportedTopologies = vpool.SupportedTopologies
			}

			qosPolicy := d.Config.QosPolicy
			if vpool.QosPolicy != "" {
				qosPolicy = vpool.QosPolicy
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes()[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
			pool.Attributes()[sa.ProvisioningType] = sa.NewStringOffer("thin")
			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[ExportPolicy] = exportPolicy
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[QosPolicy] = qosPolicy
			pool.InternalAttributes()[Region] = region
			pool.InternalAttributes()[Zone] = zone

			pool.SetSupportedTopologies(supportedTopologies)

			d.pools[pool.Name()] = pool
		}
	}

	return nil
}

// initializeAstraDSConfig parses the AstraDS config, mixing in the specified common config.
func (d *StorageDriver) initializeAstraDSConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (*drivers.AstraDSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAstraDSConfig", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAstraDSConfig")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAstraDSConfig")
	}

	config := &drivers.AstraDSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AstraDSStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		err = config.InjectSecrets(backendSecret)
		if err != nil {
			return nil, fmt.Errorf("could not inject backend secret; %v", err)
		}
	}

	return config, nil
}

// validate ensures the driver configuration and execution environment are valid and working
func (d *StorageDriver) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	// Validate AstraDS cluster version
	if clusterVersion, err := utils.ParseSemantic(d.API.Cluster.Version); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"cluster": d.API.Cluster.Name,
			"version": d.API.Cluster.Version,
		}).WithError(err).Error("AstraDS cluster version is invalid.")
	} else {
		if !clusterVersion.AtLeast(utils.MustParseSemantic(minimumAstraDSVersion)) {
			return fmt.Errorf("AstraDS cluster version is %s, at least %s is required",
				clusterVersion.String(), minimumAstraDSVersion)
		}
	}

	// Validate driver-level attributes

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Ensure Docker is only used with static export policies
	if d.Config.DriverContext == tridentconfig.ContextDocker && d.Config.AutoExportPolicy {
		return fmt.Errorf("automatic export policies are not supported with Docker")
	}

	// Ensure config has a set of valid autoExportCIDRs
	if err := utils.ValidateCIDRs(ctx, d.Config.AutoExportCIDRs); err != nil {
		return fmt.Errorf("failed to validate auto-export CIDR(s): %w", err)
	}

	// Build list of extant QoS policies on the cluster
	qosPolicyNames := make([]string, 0)
	if qosPolicies, err := d.API.GetQosPolicies(ctx); err != nil {
		return fmt.Errorf("could not read AstraDS QoS policies; %v", err)
	} else {
		for _, qosPolicy := range qosPolicies {
			qosPolicyNames = append(qosPolicyNames, qosPolicy.Name)
		}
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate snapshot dir
		if pool.InternalAttributes()[SnapshotDir] != "" {
			_, err := strconv.ParseBool(pool.InternalAttributes()[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s; %v", poolName, err)
			}
		}

		// Validate default size
		if _, err := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s; %v", poolName, err)
		}

		// Validate QoS policy
		qosPolicyName := pool.InternalAttributes()[QosPolicy]
		if qosPolicyName != "" && !utils.SliceContainsString(qosPolicyNames, qosPolicyName) {
			return fmt.Errorf("QoS policy %s does not exist", qosPolicyName)
		}

		// Validate static export policy
		if !d.Config.AutoExportPolicy {
			exportPolicy := pool.InternalAttributes()[ExportPolicy]
			if exists, _, err := d.API.ExportPolicyExists(ctx, exportPolicy); err != nil {
				return fmt.Errorf("could not check for export policy; %v", err)
			} else if !exists {
				return fmt.Errorf("export policy %s does not exist", exportPolicy)
			}
		}

		if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, maxAnnotationLength); err != nil {
			return fmt.Errorf("invalid value for label in pool %s; %v", poolName, err)
		}
	}

	return nil
}

// Create creates a new empty volume with the specified options
func (d *StorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool,
	volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "StorageDriver",
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

	// If the volume already exists, check its condition.  If OK, add the finalizer and return success.
	// If the creation has failed, delete the CR and return the creation error.  If still creating,
	// return VolumeCreatingError to enable retries.
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("could not check for existing volume; %v", err)
	}
	if volumeExists {

		if creationErr := extantVolume.GetCreationError(); creationErr != nil {
			// If we failed to create the volume, clean up the CR(s)
			d.cleanUpFailedVolume(ctx, extantVolume)
			return fmt.Errorf("volume %s creation failed; %v", extantVolume.Name, creationErr)
		}

		if !extantVolume.IsReady(ctx) {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(fmt.Sprintf("volume %s is still not ready", name))
		}

		// The creation succeeded, so place a finalizer on the volume
		if err = d.API.SetVolumeAttributes(ctx, extantVolume,
			roaring.BitmapOf(api.UpdateFlagAddFinalizer)); err != nil {
			Logc(ctx).WithField("volume", extantVolume.Name).WithError(err).Error("Could not add finalizer to volume.")
		}

		return drivers.NewVolumeExistsError(name)
	}

	// Take snapshot directory from volume config first (handles Docker case & PVC annotations), then from pool
	snapshotReserve := volConfig.SnapshotReserve
	if snapshotReserve == "" {
		snapshotReserve = pool.InternalAttributes()[SnapshotReserve]
	}
	snapshotReserveInt64, err := strconv.ParseInt(snapshotReserve, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}
	snapshotReserveInt32 := int32(snapshotReserveInt64)
	if snapshotReserveInt32 < minimumSnapshotReserve || snapshotReserveInt32 > maximumSnapshotReserve {
		return fmt.Errorf("snapshotReserve must be in the range [%d, %d]",
			minimumSnapshotReserve, maximumSnapshotReserve)
	}

	// Take snapshot directory from volume config first (handles Docker case & PVC annotations), then from pool
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes()[SnapshotDir]
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir; %v", err)
	}

	// Take export policy from volume config first (handles Docker case & PVC annotations), then from pool
	exportPolicy := volConfig.ExportPolicy
	if exportPolicy == "" {
		exportPolicy = pool.InternalAttributes()[ExportPolicy]
	}
	// Override export policy if using an automatic one
	if d.Config.AutoExportPolicy {
		exportPolicy = name // Per-volume export policy can share volume's name
	}

	// Take unix permissions from volume config first (handles Docker case & PVC annotations), then from pool
	unixPermissions := volConfig.UnixPermissions
	if unixPermissions == "" {
		unixPermissions = pool.InternalAttributes()[UnixPermissions]
	}

	qosPolicy := storagePool.InternalAttributes()[QosPolicy]
	volConfig.QosPolicy = qosPolicy

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
	if err := drivers.CheckMinVolumeSize(sizeBytes, MinimumVolumeSizeBytes); err != nil {
		return err
	}
	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Get the adjusted volume size based on the snapshot reserve
	adjustedSizeBytes := d.padVolumeSizeWithSnapshotReserve(ctx, name, sizeBytes, snapshotReserveInt32)

	// Build our JSON-formatted metadata, which we will set as CR annotations
	poolLabelsJSON, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, 256*1024)
	if err != nil {
		return fmt.Errorf("could not build volume labels; %v", err)
	}
	annotations := map[string]string{
		drivers.TridentLabelTag:      d.getTelemetryLabelsJSON(ctx),
		storage.ProvisioningLabelTag: poolLabelsJSON,
	}

	newVolume := &api.Volume{
		Name:            name,
		Namespace:       d.Config.Namespace,
		Annotations:     annotations,
		Labels:          d.validateLabels(ctx, pool.GetLabels(ctx, labelPrefix)),
		DisplayName:     volConfig.Name,
		RequestedSize:   resource.MustParse(strconv.FormatInt(int64(adjustedSizeBytes), 10)),
		Type:            api.NetappVolumeTypeReadWrite,
		ExportPolicy:    exportPolicy,
		QoSPolicy:       qosPolicy,
		VolumePath:      "/" + name,
		Permissions:     unixPermissions,
		SnapshotReserve: snapshotReserveInt32,
		NoSnapDir:       !snapshotDirBool,
	}

	// Ensure the export policy exists
	if d.Config.AutoExportPolicy {
		if _, err = d.API.EnsureExportPolicyExists(ctx, exportPolicy); err != nil {
			Logc(ctx).WithField("exportPolicy", exportPolicy).WithError(err).Error(
				"Could not ensure export policy exists.")
			return fmt.Errorf("could not ensure export policy exists; %v", err)
		}
	} else {
		if exists, _, err := d.API.ExportPolicyExists(ctx, exportPolicy); err != nil {
			Logc(ctx).WithField("exportPolicy", exportPolicy).WithError(err).Error(
				"Could not check for export policy.")
			return fmt.Errorf("could not check for export policy; %v", err)
		} else if !exists {
			return fmt.Errorf("export policy %s does not exist", exportPolicy)
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"name":            name,
		"size":            sizeBytes,
		"adjustedSize":    adjustedSizeBytes,
		"exportPolicy":    exportPolicy,
		"snapshotReserve": snapshotReserveInt32,
		"snapshotDir":     snapshotDirBool,
		"unixPermissions": unixPermissions,
		"qosPolicy":       qosPolicy,
	}).Debug("Creating volume.")

	// Create the volume
	newVolume, err = d.API.CreateVolume(ctx, newVolume)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	if err = d.API.WaitForVolumeReady(ctx, newVolume, d.volumeCreateTimeout); err != nil {

		// If volume is still creating, return creating error to give it more time
		if utils.IsVolumeCreatingError(err) {
			return err
		}

		// If we failed to create the volume, clean up the CR(s)
		d.cleanUpFailedVolume(ctx, newVolume)

		return err
	}

	// The creation succeeded, so place a finalizer on the volume
	if err = d.API.SetVolumeAttributes(ctx, newVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)); err != nil {
		Logc(ctx).WithField("volume", newVolume.Name).WithError(err).Error("Could not add finalizer to volume.")
	}

	Logc(ctx).WithField("name", name).Info("Created volume.")

	return nil
}

// padVolumeSizeWithSnapshotReserve calculates the size of the volume taking into account the snapshot reserve
func (d *StorageDriver) padVolumeSizeWithSnapshotReserve(
	ctx context.Context, volumeName string, requestedSize uint64, snapshotReserve int32,
) uint64 {

	if snapshotReserve < 0 || snapshotReserve > 90 {

		Logc(ctx).WithFields(log.Fields{
			"volume":          volumeName,
			"snapshotReserve": snapshotReserve,
		}).Error("Invalid snapshot reserve value.")

		return requestedSize
	}

	snapReserveDivisor := 1.0 - (float64(snapshotReserve) / 100.0)

	sizeWithSnapReserve := uint64(float64(requestedSize) / snapReserveDivisor)

	Logc(ctx).WithFields(log.Fields{
		"volume":              volumeName,
		"snapReserveDivisor":  snapReserveDivisor,
		"requestedSize":       requestedSize,
		"sizeWithSnapReserve": sizeWithSnapReserve,
	}).Debug("Calculated optimal size for volume with snapshot reserve.")

	return sizeWithSnapReserve
}

// unpadVolumeSizeWithSnapshotReserve calculates the size of the volume taking into account the snapshot reserve
func (d *StorageDriver) unpadVolumeSizeWithSnapshotReserve(
	ctx context.Context, volumeName string, actualSize uint64, snapshotReserve int32,
) uint64 {

	if snapshotReserve < 0 || snapshotReserve > 90 {

		Logc(ctx).WithFields(log.Fields{
			"volume":          volumeName,
			"snapshotReserve": snapshotReserve,
		}).Error("Invalid snapshot reserve value.")

		return actualSize
	}

	snapReserveMultiplier := 1.0 - (float64(snapshotReserve) / 100.0)

	sizeWithoutSnapReserve := uint64(float64(actualSize) * snapReserveMultiplier)

	Logc(ctx).WithFields(log.Fields{
		"volume":                 volumeName,
		"snapReserveMultiplier":  snapReserveMultiplier,
		"actualSize":             actualSize,
		"sizeWithoutSnapReserve": sizeWithoutSnapReserve,
	}).Debug("Calculated usable size for volume with snapshot reserve.")

	return sizeWithoutSnapReserve
}

// CreateClone creates a volume from an existing volume or snapshot.
func (d *StorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, pool storage.Pool,
) error {

	name := cloneVolConfig.InternalName
	sourceVolumeName := cloneVolConfig.CloneSourceVolumeInternal
	sourceSnapshotName := cloneVolConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":         "CreateClone",
			"Type":           "StorageDriver",
			"name":           name,
			"sourceVolume":   sourceVolumeName,
			"sourceSnapshot": sourceSnapshotName,
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

	// If the volume already exists, check its condition.  If OK, add the finalizer and return success.
	// If the creation has failed, delete the CR and return the creation error.  If still creating,
	// return VolumeCreatingError to enable retries.
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("could not check for existing volume; %v", err)
	}
	if volumeExists {

		if creationErr := extantVolume.GetCreationError(); creationErr != nil {
			// If we failed to create the volume, clean up the CR(s)
			d.cleanUpFailedVolume(ctx, extantVolume)
			return fmt.Errorf("volume %s creation failed; %v", extantVolume.Name, creationErr)
		}

		if !extantVolume.IsReady(ctx) {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(fmt.Sprintf("volume %s is still not ready", name))
		}

		// The creation succeeded, so place a finalizer on the volume
		if err = d.API.SetVolumeAttributes(ctx, extantVolume,
			roaring.BitmapOf(api.UpdateFlagAddFinalizer)); err != nil {
			Logc(ctx).WithField("volume", extantVolume.Name).WithError(err).Error("Could not add finalizer to volume.")
		}

		return drivers.NewVolumeExistsError(name)
	}

	// Get the source volume
	sourceVolume, err := d.API.GetVolume(ctx, sourceVolumeName)
	if err != nil {
		return fmt.Errorf("could not find source volume; %v", err)
	}

	// Get the source snapshot, if specified
	if sourceSnapshotName != "" {
		_, err := d.API.GetSnapshot(ctx, sourceSnapshotName)
		if err != nil {
			return fmt.Errorf("could not find source snapshot; %v", err)
		}
	}

	// Take export policy from volume config first (handles Docker case & PVC annotations), then from source volume
	exportPolicy := cloneVolConfig.ExportPolicy
	if exportPolicy == "" {
		exportPolicy = sourceVolume.ExportPolicy
	}
	// Override export policy if using an automatic one
	if d.Config.AutoExportPolicy {
		exportPolicy = name // Per-volume export policy can share volume's name
	}

	// Handle unknown pool, since a pool is needed for labels & annotations
	if pool == nil || pool.Name() == drivers.UnsetPool {
		// We don't know the source pool, so use what we can from the config
		pool = &storage.StoragePool{}
		pool.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.Config.Labels),
		})
	}

	// Build our JSON-formatted metadata, which we will set as CR annotations
	poolLabelsJSON, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, 256*1024)
	if err != nil {
		return fmt.Errorf("could not build volume labels; %v", err)
	}
	annotations := map[string]string{
		drivers.TridentLabelTag:      d.getTelemetryLabelsJSON(ctx),
		storage.ProvisioningLabelTag: poolLabelsJSON,
	}

	cloneVolume := &api.Volume{
		Name:            name,
		Namespace:       d.Config.Namespace,
		Annotations:     annotations,
		Labels:          d.validateLabels(ctx, pool.GetLabels(ctx, labelPrefix)),
		DisplayName:     cloneVolConfig.Name,
		RequestedSize:   sourceVolume.RequestedSize,
		Type:            api.NetappVolumeTypeReadWrite,
		ExportPolicy:    exportPolicy,
		QoSPolicy:       sourceVolume.QoSPolicy,
		VolumePath:      "/" + name,
		Permissions:     sourceVolume.Permissions,
		SnapshotReserve: sourceVolume.SnapshotReserve,
		NoSnapDir:       sourceVolume.NoSnapDir,
	}

	// Only specify source volume or snapshot, not both
	if sourceSnapshotName != "" {
		cloneVolume.CloneSnapshot = sourceSnapshotName
	} else {
		cloneVolume.CloneVolume = sourceVolumeName
	}

	// Ensure the export policy exists
	if d.Config.AutoExportPolicy {
		if _, err = d.API.EnsureExportPolicyExists(ctx, exportPolicy); err != nil {
			Logc(ctx).WithField("exportPolicy", exportPolicy).WithError(err).Error(
				"Could not ensure export policy exists.")
			return fmt.Errorf("could not ensure export policy exists; %v", err)
		}
	} else {
		if exists, _, err := d.API.ExportPolicyExists(ctx, exportPolicy); err != nil {
			Logc(ctx).WithField("exportPolicy", exportPolicy).WithError(err).Error(
				"Could not check for export policy.")
			return fmt.Errorf("could not check for export policy; %v", err)
		} else if !exists {
			return fmt.Errorf("export policy %s does not exist", exportPolicy)
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"name":            name,
		"sourceVolume":    sourceVolume.Name,
		"sourceSnapshot":  sourceSnapshotName,
		"size":            sourceVolume.RequestedSize,
		"exportPolicy":    exportPolicy,
		"snapshotReserve": sourceVolume.SnapshotReserve,
		"snapshotDir":     !sourceVolume.NoSnapDir,
		"unixPermissions": sourceVolume.Permissions,
		"qosPolicy":       sourceVolume.QoSPolicy,
	}).Debug("Cloning volume.")

	// Create the volume
	cloneVolume, err = d.API.CreateVolume(ctx, cloneVolume)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	if err = d.API.WaitForVolumeReady(ctx, cloneVolume, d.volumeCreateTimeout); err != nil {

		// If volume is still creating, return creating error to give it more time
		if utils.IsVolumeCreatingError(err) {
			return err
		}

		// If we failed to create the volume, clean up the CR(s)
		d.cleanUpFailedVolume(ctx, cloneVolume)

		return err
	}

	// The creation succeeded, so place a finalizer on the volume
	if err = d.API.SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)); err != nil {
		Logc(ctx).WithField("volume", cloneVolume.Name).WithError(err).Error("Could not add finalizer to volume.")
	}

	Logc(ctx).WithField("name", name).Info("Created volume clone.")

	return nil
}

// cleanUpFailedVolume is called when volume creation fails.  It cleans up the volume CR and any automatic
// export policy CR so that nothing is left behind.  In such cases we are already returning the volume creation
// error from the workflow, so these cleanup operations merely log any errors they encounter.
func (d *StorageDriver) cleanUpFailedVolume(ctx context.Context, volume *api.Volume) {

	// If we failed to create a volume, clean up the volume CR
	if deleteErr := d.API.DeleteVolume(ctx, volume); deleteErr != nil {
		Logc(ctx).WithField("volume", volume.Name).WithError(deleteErr).Error("Could not delete failed volume.")
	} else {
		Logc(ctx).WithField("volume", volume.Name).Warning("Deleted failed volume.")
	}

	// If we didn't create an automatic export policy, we're done.
	if !d.Config.AutoExportPolicy {
		return
	}

	// Take the export policy name from the volume status, but that could be blank if the volume was never
	// created, so fall back to the volume name which should always match.
	exportPolicy := volume.ExportPolicy
	if exportPolicy == "" {
		exportPolicy = volume.Name
	}

	// If we failed to create the volume *and* we created an automatic export policy, clean up the export policy CR
	var err error
	d.destroyExportPolicy(ctx, exportPolicy, &err)
}

// Import brings an existing AstraDS volume under Trident management.
func (d *StorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "StorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Import")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
	}

	// Ensure volume exists and may be imported
	volumeExists, volume, err := d.API.VolumeExists(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not check for existing volume; %v", err)
	} else if !volumeExists {
		return fmt.Errorf("volume %s does not exist; %v", err, originalName)
	} else if !volume.DeletionTimestamp.IsZero() {
		return fmt.Errorf("volume %s is being deleted; %v", err, originalName)
	}

	// Account for snapshot reserve to get the usable volume size
	actualSize := uint64(volume.ActualSize.Value())
	usableSize := d.unpadVolumeSizeWithSnapshotReserve(ctx, volume.Name, actualSize, volume.SnapshotReserve)

	volConfig.Size = strconv.FormatUint(usableSize, 10)

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {

		if volume.Annotations == nil {
			volume.Annotations = make(map[string]string)
		}
		volume.Annotations[drivers.TridentLabelTag] = d.getTelemetryLabelsJSON(ctx)
		delete(volume.Annotations, storage.ProvisioningLabelTag)

		// Update volume annotations
		if err = d.API.SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAnnotations)); err != nil {
			err = fmt.Errorf("could not set annotations for volume %s: %v", volume.Name, err)
			Logc(ctx).WithField("volume", volume.Name).WithError(err).Error("Could not import volume.")
			return err
		}
	}

	// The CR name cannot be changed, so use it as the internal name
	volConfig.InternalName = originalName

	Logc(ctx).WithFields(log.Fields{
		"name":       originalName,
		"noManage":   volConfig.ImportNotManaged,
		"actualSize": actualSize,
		"usableSize": usableSize,
	}).Info("Imported volume.")

	return nil
}

// Rename changes the name of a volume (not supported by AstraDS).
func (d *StorageDriver) Rename(ctx context.Context, name, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "StorageDriver",
			"name":    name,
			"newName": newName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Rename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Rename")
	}

	// Rename is only needed for the import workflow, and we aren't currently renaming the
	// AstraDS volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}

// getTelemetryLabelsJSON builds the labels that are set on each volume.
func (d *StorageDriver) getTelemetryLabelsJSON(ctx context.Context) string {

	telemetry := Telemetry{
		Telemetry: tridentconfig.OrchestratorTelemetry,
		Plugin:    d.Name(),
	}

	telemetryMap := map[string]Telemetry{drivers.TridentLabelTag: telemetry}

	telemetryJSON, err := json.Marshal(telemetryMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal telemetry: %+v", telemetry)
	}

	return strings.ReplaceAll(string(telemetryJSON), " ", "")
}

// validateLabels accepts a map of labels and returns the same map with all invalid labels removed.
func (d *StorageDriver) validateLabels(ctx context.Context, labels map[string]string) map[string]string {

	validLabels := make(map[string]string)

	for k, v := range labels {
		if len(validation.IsQualifiedName(k)) > 0 {
			Logc(ctx).Warningf("Ignoring invalid label: (%s: %s)", k, v)
			continue
		}
		validLabels[k] = v
	}

	return validLabels
}

// Destroy deletes a volume.
func (d *StorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) (err error) {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "StorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	defer d.destroyExportPolicy(ctx, name, &err)

	// If volume doesn't exist, return success
	volumeExists, extantVolume, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("could not check for existing volume; %v", err)
	} else if !volumeExists {
		Logc(ctx).WithField("volume", name).Warning("Volume already deleted.")
		return nil
	}

	// Remove finalizer from the volume
	if err = d.API.SetVolumeAttributes(ctx, extantVolume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)); err != nil {
		Logc(ctx).WithField("volume", extantVolume.Name).WithError(err).Error("Could not remove finalizer from volume.")
		return err
	}

	// If volume is still deleting, don't wait for it.
	if !extantVolume.DeletionTimestamp.IsZero() {
		Logc(ctx).WithField("volume", name).Warning("Volume is still deleting.")
		return fmt.Errorf("volume %s is still deleting", name)
	}

	// Delete the volume
	if err = d.API.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	// Wait for deletion to complete
	if err = d.API.WaitForVolumeDeleted(ctx, extantVolume, d.defaultDeleteTimeout()); err != nil {
		return err
	}

	Logc(ctx).WithField("name", name).Info("Volume deleted.")

	return nil
}

// destroyExportPolicy is a deferred function called from Destroy() that cleans up any export policy whose
// lifecycle was tied to that of a volume.
func (d *StorageDriver) destroyExportPolicy(ctx context.Context, name string, destroyErr *error) {

	// If the volume delete failed, it will be retried, so don't delete the export policy yet either.
	if *destroyErr != nil {
		return
	}

	// The export policy shouldn't be empty, but ignore if it is
	if name == "" {
		Logc(ctx).WithField("exportPolicy", name).Warning("Export policy is empty, nothing to delete.")
		return
	}

	// Ensure export policy isn't shared at either the config or virtual pool level
	if name == d.Config.ExportPolicy {
		Logc(ctx).WithField("exportPolicy", name).Debug("Not deleting shared export policy.")
		return
	}
	for _, pool := range d.pools {
		if name == pool.InternalAttributes()[ExportPolicy] {
			Logc(ctx).WithField("exportPolicy", name).Debug("Not deleting shared virtual pool export policy.")
			return
		}
	}

	// Check if an export policy exists
	exists, exportPolicy, err := d.API.ExportPolicyExists(ctx, name)
	if err != nil {
		Logc(ctx).WithField("exportPolicy", name).WithError(err).Warning("Could not check for export policy.")
		return
	} else if !exists {
		Logc(ctx).WithField("exportPolicy", name).Debug("Export policy does not exist, nothing to delete.")
		return
	}

	// Remove finalizer from the export policy
	err = d.API.SetExportPolicyAttributes(ctx, exportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer))
	if err != nil {
		Logc(ctx).WithField("exportPolicy", name).WithError(err).Error("Could not remove finalizer from export policy.")
		return
	}

	// Delete the export policy
	if err = d.API.DeleteExportPolicy(ctx, name); err != nil {
		Logc(ctx).WithField("exportPolicy", name).WithError(err).Error("Could not delete export policy.")
	} else {
		Logc(ctx).WithField("exportPolicy", name).Info("Export policy deleted.")
	}
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *StorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "StorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	volume, err := d.API.GetVolume(ctx, name)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsServerIP = volume.ExportAddress
	publishInfo.NfsPath = "/" + strings.TrimPrefix(volume.VolumePath, "/")
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	err = d.publishNFSShare(ctx, volConfig, publishInfo, volume)
	if err != nil {
		Logc(ctx).WithField("name", volume.Name).WithError(err).Error("Could not publish volume.")
	} else {
		Logc(ctx).WithField("name", volume.Name).Info("Published volume.")
	}
	return err
}

// publishNFSShare ensures that the volume has the correct export policy applied
// along with the needed access rules.
func (d *StorageDriver) publishNFSShare(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo, volume *api.Volume,
) error {

	name := volume.Name
	policyName := volume.Name

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "publishNFSShare",
			"Type":   "StorageDriver",
			"Share":  name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> publishNFSShare")
		defer Logc(ctx).WithFields(fields).Debug("<<<< publishNFSShare")
	}

	if !d.Config.AutoExportPolicy || volConfig.ImportNotManaged {
		// Nothing to do if we're not configuring export policies automatically or volume is not managed
		return nil
	}

	// Ensure the export policy exists and has the correct rule set
	if err := d.grantNodeAccess(ctx, publishInfo, policyName); err != nil {
		return err
	}

	// Return if volume already has the correct export policy
	if volume.ExportPolicy == volume.Name {
		return nil
	}

	// Set the export policy to the volume name
	volume.ExportPolicy = volume.Name

	// Update volume to use the correct export policy
	if err := d.API.SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)); err != nil {
		err = fmt.Errorf("could not set export policy for volume %s: %v", name, err)
		Logc(ctx).WithField("volume", volume.Name).WithError(err).Error("Could not set export policy for volume.")
		return err
	}

	Logc(ctx).WithField("name", volume.Name).Info("Set export policy for volume.")

	return nil
}

// grantNodeAccess checks to see if the export policy exists and if not it will create it.  Then it ensures
// that the IPs in the publish info are reflected as rules on the export policy.
func (d *StorageDriver) grantNodeAccess(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, policyName string,
) error {

	exportPolicy, err := d.API.EnsureExportPolicyExists(ctx, policyName)
	if err != nil {
		err = fmt.Errorf("unable to ensure export policy exists; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not grant node access.")
		return err
	}

	Logc(ctx).WithField("name", policyName).Debug("Export policy exists.")

	addedIPs, err := utils.FilterIPs(ctx, publishInfo.HostIP, d.Config.AutoExportCIDRs)
	if err != nil {
		err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not grant node access.")
		return err
	}

	err = d.reconcileExportPolicyRules(ctx, exportPolicy, addedIPs, nil)
	if err != nil {
		err = fmt.Errorf("unable to reconcile export policy rules; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not grant node access.")
		return err
	}

	return nil
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *StorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, nodes []*utils.Node,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Unpublish",
			"Type":   "StorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Unpublish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Unpublish")
	}

	volume, err := d.API.GetVolume(ctx, name)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	err = d.unpublishNFSShare(ctx, volConfig, nodes, volume)
	if err != nil {
		Logc(ctx).WithField("name", volume.Name).WithError(err).Error("Could not unpublish volume.")
	} else {
		Logc(ctx).WithField("name", volume.Name).Info("Unpublished volume.")
	}
	return err
}

// unpublishNFSShare ensures that the volume does not have access to a node to which is has been unpublished.
// The node list represents the set of nodes to which the volume should be published.
func (d *StorageDriver) unpublishNFSShare(
	ctx context.Context, volConfig *storage.VolumeConfig, nodes []*utils.Node, volume *api.Volume,
) error {

	name := volume.Name
	policyName := volume.Name

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "unpublishNFSShare",
			"Type":   "StorageDriver",
			"Share":  name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> unpublishNFSShare")
		defer Logc(ctx).WithFields(fields).Debug("<<<< unpublishNFSShare")
	}

	if !d.Config.AutoExportPolicy || volConfig.ImportNotManaged {
		// Nothing to do if we're not configuring export policies automatically or volume is not managed
		return nil
	}

	// Ensure the export policy exists and has the correct rule set
	if err := d.setNodeAccess(ctx, nodes, policyName); err != nil {
		return err
	}

	// Return if volume already has the correct export policy
	if volume.ExportPolicy == volume.Name {
		return nil
	}

	// Set the export policy to the volume name
	volume.ExportPolicy = volume.Name

	// Update volume to use the correct export policy
	if err := d.API.SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)); err != nil {
		err = fmt.Errorf("could not set export policy for volume %s: %v", name, err)
		Logc(ctx).WithField("volume", volume.Name).WithError(err).Error("Could not set export policy for volume.")
		return err
	}

	Logc(ctx).WithField("name", volume.Name).Info("Set export policy for volume.")

	return nil
}

// setNodeAccess checks to see if the export policy exists and if not it will create it.  Then it ensures
// that the IPs in the node list exactly match the rules on the export policy.
func (d *StorageDriver) setNodeAccess(
	ctx context.Context, nodes []*utils.Node, policyName string,
) error {

	exportPolicy, err := d.API.EnsureExportPolicyExists(ctx, policyName)
	if err != nil {
		err = fmt.Errorf("unable to ensure export policy exists; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not set node access.")
		return err
	}

	allIPs := make([]string, 0)

	for _, node := range nodes {
		nodeIPs, ipErr := utils.FilterIPs(ctx, node.IPs, d.Config.AutoExportCIDRs)
		if ipErr != nil {
			err = fmt.Errorf("unable to determine undesired export policy rules; %v", err)
			Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not set node access.")
			return err
		}

		allIPs = append(allIPs, nodeIPs...)
	}

	err = d.setExportPolicyRules(ctx, exportPolicy, allIPs)
	if err != nil {
		err = fmt.Errorf("unable to set export policy rules; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).WithError(err).Error("Could not set node access.")
		return err
	}

	return nil
}

// reconcileExportPolicyRules updates a set of access rules on an export policy.
func (d *StorageDriver) reconcileExportPolicyRules(
	ctx context.Context, policy *api.ExportPolicy, addedIPs []string, removedIPs []string,
) error {

	// Start with existing rules
	desiredPolicyRules := make(map[string]bool)
	for _, rule := range policy.Rules {
		for _, client := range rule.Clients {
			desiredPolicyRules[client] = true
		}
	}

	// Purge map of removed IPs
	for _, ip := range removedIPs {
		delete(desiredPolicyRules, ip)
	}

	// Update map with added IPs
	for _, ip := range addedIPs {
		desiredPolicyRules[ip] = true
	}

	// Replace the rules on the export policy
	policy.Rules = make([]api.ExportPolicyRule, 0)
	index := uint64(1)

	for ip := range desiredPolicyRules {

		policy.Rules = append(policy.Rules, api.ExportPolicyRule{
			Clients:   []string{ip},
			Protocols: []string{"nfs4"},
			RuleIndex: index,
			RoRules:   []string{"any"},
			RwRules:   []string{"any"},
			SuperUser: []string{"any"},
			AnonUser:  65534,
		})

		index++
	}

	if err := d.API.SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)); err != nil {
		Logc(ctx).WithField("name", policy.Name).WithError(err).Error("Could not update export policy rules.")
		return err
	}

	Logc(ctx).WithField("name", policy.Name).Info("Updated export policy rules.")

	return nil
}

// setExportPolicyRules replaces a set of access rules on an export policy to the specified set.
func (d *StorageDriver) setExportPolicyRules(ctx context.Context, policy *api.ExportPolicy, IPs []string) error {

	// Replace the rules on the export policy
	policy.Rules = make([]api.ExportPolicyRule, 0)
	index := uint64(1)

	for _, ip := range IPs {

		policy.Rules = append(policy.Rules, api.ExportPolicyRule{
			Clients:   []string{ip},
			Protocols: []string{"nfs4"},
			RuleIndex: index,
			RoRules:   []string{"any"},
			RwRules:   []string{"any"},
			SuperUser: []string{"any"},
			AnonUser:  65534,
		})

		index++
	}

	if err := d.API.SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)); err != nil {
		Logc(ctx).WithField("name", policy.Name).WithError(err).Error("Could not update export policy rules.")
		return err
	}

	Logc(ctx).WithField("name", policy.Name).Info("Updated export policy rules.")

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *StorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *StorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "StorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return d.getSnapshot(ctx, snapConfig)
}

// getSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *StorageDriver) getSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	// Get the snapshot
	snapshot, err := d.API.GetSnapshot(ctx, internalSnapName)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}

	// Double check we got the right snapshot
	if internalVolName != snapshot.VolumeName {
		return nil, fmt.Errorf("snapshot %s exists on a different volume", internalSnapName)
	}

	created := snapshot.FormatCreationTime()

	state := storage.SnapshotStateOnline
	if !snapshot.ReadyToUse {
		state = storage.SnapshotStateCreating
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      created,
		"state":        state,
	}).Debug("Found snapshot.")

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   created,
		SizeBytes: 0,
		State:     state,
	}, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *StorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {

	internalVolName := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "StorageDriver",
			"volumeName": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	// Get the volume
	volume, err := d.API.GetVolume(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s; %v", internalVolName, err)
	}

	snapshots, err := d.API.GetSnapshots(ctx, volume)
	if err != nil {
		return nil, fmt.Errorf("could not list snapshots for volume %s; %v", internalVolName, err)
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, snapshot := range snapshots {

		// Filter out snapshots that aren't ready
		if !snapshot.ReadyToUse {
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
			Created:   snapshot.FormatCreationTime(),
			SizeBytes: 0,
			State:     storage.SnapshotStateOnline,
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *StorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "StorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	// Check if volume exists
	volumeExists, _, err := d.API.VolumeExists(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("could not check for existing volume; %v", err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Check if snapshot exists
	snapshotExists, _, err := d.API.SnapshotExists(ctx, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not check for existing snapshot; %v", err)
	}
	if snapshotExists {
		return nil, fmt.Errorf("snapshot %s already exists", internalSnapName)
	}

	newSnapshot := &api.Snapshot{
		Name:       internalSnapName,
		Namespace:  d.Config.Namespace,
		VolumeName: internalVolName,
	}

	// Create the snapshot
	newSnapshot, err = d.API.CreateSnapshot(ctx, newSnapshot)
	if err != nil {
		return nil, err
	}

	logFields := log.Fields{
		"volume":   newSnapshot.VolumeName,
		"snapshot": newSnapshot.Name,
	}

	// Wait a short time for snapshot to become ready
	if err = d.API.WaitForSnapshotReady(ctx, newSnapshot, snapshotCreateTimeout); err != nil {

		// If we failed to create the snapshot, clean up the CR
		d.cleanUpFailedSnapshot(ctx, newSnapshot)

		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Snapshot created.")

	// The creation succeeded, so place a finalizer on the snapshot
	if err = d.API.SetSnapshotAttributes(ctx, newSnapshot, roaring.BitmapOf(api.UpdateFlagAddFinalizer)); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not add finalizer to snapshot.")
	}

	return d.getSnapshot(ctx, snapConfig)
}

// cleanUpFailedSnapshot is called when snapshot creation fails.  It cleans up the snapshot CR so that nothing
// is left behind.  In such cases we are already returning the snapshot creation error from the workflow, so
// these cleanup operations merely log any errors they encounter.
func (d *StorageDriver) cleanUpFailedSnapshot(ctx context.Context, snapshot *api.Snapshot) {

	// If we failed to create a snapshot, clean up the snapshot CR
	if deleteErr := d.API.DeleteSnapshot(ctx, snapshot); deleteErr != nil {
		Logc(ctx).WithField("snapshot", snapshot.Name).WithError(deleteErr).Error("Could not delete failed snapshot.")
	} else {
		Logc(ctx).WithField("snapshot", snapshot.Name).Warning("Deleted failed snapshot.")
	}
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *StorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "StorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return utils.UnsupportedError(fmt.Sprintf("snapshot restores are not supported by backend type %s", d.Name()))
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *StorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "StorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	logFields := log.Fields{
		"volume":   internalVolName,
		"snapshot": internalSnapName,
	}

	// Check if snapshot exists and hasn't already been deleted.
	snapshotExists, extantSnapshot, err := d.API.SnapshotExists(ctx, internalSnapName)
	if err != nil {
		return fmt.Errorf("could not check for existing snapshot; %v", err)
	} else if !snapshotExists {
		Logc(ctx).WithFields(logFields).Warning("Snapshot already deleted.")
		return nil
	}

	// Remove finalizer from the snapshot
	if err = d.API.SetSnapshotAttributes(ctx, extantSnapshot,
		roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not remove finalizer from snapshot.")
		return err
	}

	// If snapshot is still deleting, don't wait for it
	if !extantSnapshot.DeletionTimestamp.IsZero() {
		Logc(ctx).WithFields(logFields).Warning("Snapshot is still deleting.")
		return fmt.Errorf("snapshot %s of volume %s is still deleting", internalSnapName, internalVolName)
	}

	// Delete the snapshot
	if err := d.API.DeleteSnapshot(ctx, extantSnapshot); err != nil {
		return err
	}

	// Wait for deletion to complete
	if err = d.API.WaitForSnapshotDeleted(ctx, extantSnapshot, d.defaultDeleteTimeout()); err != nil {
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Snapshot deleted.")

	return nil
}

// List returns the list of volumes.
func (d *StorageDriver) List(ctx context.Context) ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> List")
		defer Logc(ctx).WithFields(fields).Debug("<<<< List")
	}

	volumes, err := d.API.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	var volumeNames []string

	for _, volume := range volumes {

		// Filter out volumes that aren't ready
		if !volume.IsReady(ctx) {
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.Name, prefix) {
			continue
		}

		volumeName := volume.Name[len(prefix):]
		volumeNames = append(volumeNames, volumeName)
	}
	return volumeNames, nil
}

// Get tests for the existence of a volume.
func (d *StorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "StorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.API.GetVolume(ctx, name)
	return err
}

// Resize increases a volume's size.
func (d *StorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "Resize",
			"Type":               "StorageDriver",
			"name":               name,
			"requestedSizeBytes": requestedSizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	volume, err := d.API.GetVolume(ctx, name)
	if err != nil {
		return fmt.Errorf("could not find volume %s; %v", name, err)
	}

	// If the volume isn't ready, return an error
	if !volume.IsReady(ctx) {
		return fmt.Errorf("volume %s is not ready", name)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	_, _, err = drivers.CheckVolumeSizeLimits(ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig)
	if err != nil {
		return err
	}

	// Adjust the new volume size to account for the snapshot reserve
	newSizeBytes := d.padVolumeSizeWithSnapshotReserve(ctx, name, requestedSizeBytes, volume.SnapshotReserve)

	// Get the size the volume should already be
	currentRequestedSizeBytes, err := volume.GetRequestedSize(ctx)
	if err != nil {
		return fmt.Errorf("could not get requested volume size %s; %v", volume.RequestedSize.String(), err)
	}

	// Make sure we're not shrinking the volume from what we have previously requested
	if int64(newSizeBytes) < currentRequestedSizeBytes {
		return utils.UnsupportedCapacityRangeError(fmt.Errorf("requested size %d is less than existing volume size %d",
			newSizeBytes, currentRequestedSizeBytes))
	}

	// If we aren't growing the volume, there's nothing to do
	if int64(newSizeBytes) == currentRequestedSizeBytes {
		volConfig.Size = strconv.FormatUint(newSizeBytes, 10)
		return nil
	}

	// Set the size
	volume.RequestedSize = resource.MustParse(strconv.FormatInt(int64(newSizeBytes), 10))

	// Resize the volume
	err = d.API.SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRequestedSize))
	if err != nil {
		return fmt.Errorf("could not resize volume %s; %v", name, err)
	}

	// Wait for resize operation to complete
	err = d.API.WaitForVolumeResize(ctx, volume, int64(newSizeBytes), volumeResizeTimeout)
	if err != nil {
		return fmt.Errorf("could not resize volume %s; %v", name, err)
	}

	Logc(ctx).WithField("name", name).Info("Resized volume.")

	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)
	return nil
}

// GetStorageBackendSpecs retrieves storage capabilities and registers pools with the specified backend.
func (d *StorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {

	backend.SetName(d.BackendName())

	for _, pool := range d.pools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	return nil
}

// CreatePrepare determines the volume's internal name.
func (d *StorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// GetStorageBackendPhysicalPoolNames retrieves the names of this backend's physical pools.
func (d *StorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return []string{}
}

// GetInternalVolumeName returns the name for a volume on the storage.  Note that this method's signature does not
// allow it to check for legality of the volume name, so that check is done during volume creation.
func (d *StorageDriver) GetInternalVolumeName(ctx context.Context, name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else if d.csiRegexp.MatchString(name) {
		// If the name is from CSI (i.e. contains a UUID), just use it as-is
		Logc(ctx).WithField("volumeInternal", name).Debug("Using volume name as internal name.")
		return name
	} else {
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		Logc(ctx).WithField("volumeInternal", internal).Debug("Modified volume name for internal name.")
		return internal
	}
}

// CreateFollowup updates the volume config with volume access details.
func (d *StorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "CreateFollowup",
			"Type":   "StorageDriver",
			"name":   volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateFollowup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateFollowup")
	}

	// Get the volume
	name := volConfig.InternalName

	volume, err := d.API.GetVolume(ctx, name)
	if err != nil {
		return fmt.Errorf("could not get volume %s; %v", name, err)
	}

	if !volume.IsReady(ctx) {
		err = fmt.Errorf("volume %s is not yet ready", name)
	}

	volConfig.AccessInfo.NfsServerIP = volume.ExportAddress
	volConfig.AccessInfo.NfsPath = "/" + strings.TrimPrefix(volume.VolumePath, "/")
	volConfig.FileSystem = "nfs"

	return nil
}

// GetProtocol returns the Trident-defined protocol supported by this driver.
func (d *StorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

// StoreConfig returns a sanitized copy of this backend's persistent config.
func (d *StorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AstraDSConfig = &d.Config
}

// GetExternalConfig returns a sanitized copy of this backends config suitable for external transmission.
func (d *StorageDriver) GetExternalConfig(ctx context.Context) interface{} {

	// Clone the config so we don't risk altering the original

	clone, err := copystructure.Copy(d.Config)
	if err != nil {
		Logc(ctx).Errorf("Could not clone AstraDS backend config; %v", err)
		return drivers.AstraDSStorageDriverConfig{}
	}

	cloneConfig, ok := clone.(drivers.AstraDSStorageDriverConfig)
	if !ok {
		Logc(ctx).Errorf("Could not cast AstraDS backend config; %v", err)
		return drivers.AstraDSStorageDriverConfig{}
	}

	cloneConfig.Kubeconfig = "<REDACTED>"
	return cloneConfig
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *StorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.GetVolume(ctx, name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(ctx, volumeAttrs), nil
}

// String implements the stringer interface for the StorageDriver driver
func (d StorageDriver) String() string {
	// Cannot use GetExternalConfig as it contains log statements
	return utils.ToStringRedacted(&d, []string{"API"}, nil)
}

// GoString implements the GoStringer interface for the StorageDriver driver
func (d StorageDriver) GoString() string {
	return d.String()
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *StorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {

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
	for _, volume := range volumes {

		// Filter out volumes that haven't finished creating yet
		if !volume.IsReady(ctx) {
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.Name, prefix) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(ctx, volume), Error: nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume as returned by the
// storage backend and formats it as a VolumeExternal object.
func (d *StorageDriver) getVolumeExternal(ctx context.Context, volume *api.Volume) *storage.VolumeExternal {

	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	sizeBytes, _ := volume.GetActualSize(ctx)

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(sizeBytes, 10),
		Protocol:        tridentconfig.File,
		ExportPolicy:    volume.ExportPolicy,
		SnapshotReserve: strconv.FormatInt(int64(volume.SnapshotReserve), 10),
		SnapshotDir:     strconv.FormatBool(!volume.NoSnapDir),
		UnixPermissions: volume.Permissions,
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver.
func (d *StorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {

	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*StorageDriver)
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

// ReconcileNodeAccess updates per-backend export policies to include access to all specified
// nodes (not supported by AstraDS).
func (d *StorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, _ string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "StorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	// This driver does not use per-backend export policies, so this method does nothing.
	return nil
}

// validateStoragePrefix ensures that the storage prefix is allowable.
func validateStoragePrefix(storagePrefix string) error {
	if storagePrefix == "" {
		return nil
	}

	matched, err := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9-]{0,62}$`, storagePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		return fmt.Errorf("storage prefix may only contain letters/digits/hyphens and must begin with a letter")
	}
	return nil
}

// GetCommonConfig returns driver's CommonConfig
func (d StorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}
