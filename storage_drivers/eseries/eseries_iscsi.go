// Copyright 2019 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries/api"
	"github.com/netapp/trident/utils"
)

const (
	DefaultHostType        = "linux_dm_mp"
	MinimumVolumeSizeBytes = 1048576 // 1 MiB

	// Constants for internal pool attributes
	Size   = "size"
	Region = "region"
	Zone   = "zone"
	Media  = "media"
)

// SANStorageDriver is for storage provisioning via the Web Services Proxy RESTful interface that communicates
// with E-Series controllers via the SYMbol API.
type SANStorageDriver struct {
	initialized bool
	Config      drivers.ESeriesStorageDriverConfig
	API         *api.Client

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *SANStorageDriver) Name() string {
	return drivers.EseriesIscsiStorageDriverName
}

// backendName returns the name of the backend managed by this driver instance
func (d *SANStorageDriver) backendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return "eseries_" + d.Config.HostDataIP
	} else {
		return d.Config.BackendName
	}
}

// poolName constructs the name of the pool reported by this driver instance
func (d *SANStorageDriver) poolName(name string) string {

	return fmt.Sprintf("%s_%s",
		d.backendName(),
		strings.Replace(name, "-", "", -1))
}

func (d *SANStorageDriver) Protocol() string {
	return "iscsi"
}

// Initialize from the provided config
func (d *SANStorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	// Trace logging hasn't been set up yet, so always do it here
	fields := log.Fields{
		"Method": "Initialize",
		"Type":   "SANStorageDriver",
	}
	log.WithFields(fields).Debug(">>>> Initialize")
	defer log.WithFields(fields).Debug("<<<< Initialize")

	commonConfig.DriverContext = context

	config := &drivers.ESeriesStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// Decode configJSON into ESeriesStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return fmt.Errorf("could not decode JSON configuration: %v", err)
	}
	d.Config = *config

	// Apply config defaults
	if err := d.populateConfigurationDefaults(config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}
	d.Config = *config

	log.WithFields(log.Fields{
		"Version":           config.Version,
		"StorageDriverName": config.StorageDriverName,
		"DebugTraceFlags":   config.DebugTraceFlags,
		"DisableDelete":     config.DisableDelete,
		"StoragePrefix":     *config.StoragePrefix,
	}).Debug("Reparsed into ESeriesStorageDriverConfig")

	telemetry := make(map[string]string)
	telemetry["version"] = tridentconfig.OrchestratorVersion.ShortString()
	telemetry["plugin"] = d.Name()
	telemetry["storagePrefix"] = *d.Config.StoragePrefix

	d.API = api.NewAPIClient(api.ClientConfig{
		WebProxyHostname:      config.WebProxyHostname,
		WebProxyPort:          config.WebProxyPort,
		WebProxyUseHTTP:       config.WebProxyUseHTTP,
		WebProxyVerifyTLS:     config.WebProxyVerifyTLS,
		Username:              config.Username,
		Password:              config.Password,
		ControllerA:           config.ControllerA,
		ControllerB:           config.ControllerB,
		PasswordArray:         config.PasswordArray,
		PoolNameSearchPattern: config.PoolNameSearchPattern,
		HostDataIP:            config.HostDataIP,
		Protocol:              d.Protocol(),
		AccessGroup:           config.AccessGroup,
		HostType:              config.HostType,
		DriverName:            config.CommonStorageDriverConfig.StorageDriverName,
		Telemetry:             telemetry,
		ConfigVersion:         config.CommonStorageDriverConfig.Version,
		DebugTraceFlags:       config.CommonStorageDriverConfig.DebugTraceFlags,
	})

	// Connect to web services proxy
	_, err = d.API.Connect()
	if err != nil {
		return fmt.Errorf("could not connect to Web Services Proxy: %v", err)
	}

	// After connected to web service, identify physical and virtual pools
	if err := d.initializeStoragePools(); err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	// Ensure the config is valid, including virtual pool config
	if err := d.validate(); err != nil {
		return fmt.Errorf("could not validate SANStorageDriver config: %v", err)
	}

	// Log chassis serial number
	chassisSerialNumber, err := d.API.GetChassisSerialNumber()
	if err != nil {
		log.Warnf("Could not determine chassis serial number. %v", err)
	} else {
		log.WithField("serialNumber", chassisSerialNumber).Info("Chassis serial number.")
		d.Config.SerialNumbers = []string{chassisSerialNumber}
	}

	// For Docker, we create a host now
	// For Kubernetes, we ensure there is a host group and warn users to populate it with hosts out of band
	// For K8S CSI, we create a host group if necessary and create the hosts automatically during the Publish calls
	if context == tridentconfig.ContextDocker {
		// Make sure this host is logged into the E-series iSCSI target
		err = utils.EnsureISCSISession(d.Config.HostDataIP)
		if err != nil {
			return fmt.Errorf("could not establish iSCSI session: %v", err)
		}

		// Make sure there is a host defined on the array for this system
		_, err = d.CreateHostForLocalHost()
		if err != nil {
			return err
		}
	} else if context == tridentconfig.ContextKubernetes {
		hostGroup, err := d.API.GetHostGroup(d.Config.AccessGroup)
		if err != nil {
			return fmt.Errorf("could not check for host group %s: %v", d.Config.AccessGroup, err)
		} else if hostGroup.ClusterRef == "" {
			return fmt.Errorf("host group %s doesn't exist for E-Series array %s and needs to be manually "+
				"created; please also ensure all relevant Hosts are defined on the array and added to the Host Group",
				d.Config.AccessGroup, d.Config.ControllerA)
		} else {
			log.WithFields(log.Fields{
				"driver":     d.Name(),
				"controller": d.Config.ControllerA,
				"hostGroup":  hostGroup.Label,
			}).Warnf("Please ensure all relevant hosts are added to Host Group %s.", d.Config.AccessGroup)
		}
	} else if context == tridentconfig.ContextCSI {
		_, err = d.API.EnsureHostGroup()
		if err != nil {
			return fmt.Errorf("could not check for host group %s: %v", d.Config.AccessGroup, err)
		}
	}

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *SANStorageDriver) populateConfigurationDefaults(config *drivers.ESeriesStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "populateConfigurationDefaults", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> populateConfigurationDefaults")
		defer log.WithFields(fields).Debug("<<<< populateConfigurationDefaults")
	}

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
	}
	if config.AccessGroup == "" {
		config.AccessGroup = drivers.GetDefaultIgroupName(config.DriverContext)
	}
	if config.HostType == "" {
		config.HostType = DefaultHostType
	}
	if config.PoolNameSearchPattern == "" {
		config.PoolNameSearchPattern = ".+"
	}

	// Fix poorly-chosen config key
	if config.HostDataIPDeprecated != "" && config.HostDataIP == "" {
		config.HostDataIP = config.HostDataIPDeprecated
	}

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	} else {
		_, err := utils.ConvertSizeToBytes(config.Size)
		if err != nil {
			return fmt.Errorf("invalid config value for default volume size: %v", err)
		}
	}

	log.WithFields(log.Fields{
		"StoragePrefix":         *config.StoragePrefix,
		"AccessGroup":           config.AccessGroup,
		"HostType":              config.HostType,
		"PoolNameSearchPattern": config.PoolNameSearchPattern,
		"Size":                  config.Size,
	}).Debugf("Configuration defaults")

	return nil
}

func (d *SANStorageDriver) initializeStoragePools() error {

	d.physicalPools = make(map[string]*storage.Pool)
	d.virtualPools = make(map[string]*storage.Pool)

	// To identify list of media types supported by physcial pools
	mediaOffers := make([]sa.Offer, 0)

	// Get pools
	physicalStoragePools, err := d.API.GetVolumePools("", 0, "")
	if err != nil {
		return fmt.Errorf("could not get storage pools from array: %v", err)
	}

	// Define physical pools
	for _, physicalStoragePool := range physicalStoragePools {

		pool := storage.NewStoragePool(nil, physicalStoragePool.Label)

		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())

		if d.Config.Region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		// E-series supports both "hdd" and "ssd" media types
		switch physicalStoragePool.DriveMediaType {
		case "hdd":
			pool.Attributes[sa.Media] = sa.NewStringOffer(sa.HDD)
			pool.InternalAttributes[Media] = sa.HDD
		case "ssd":
			pool.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
			pool.InternalAttributes[Media] = sa.SSD
		}

		if mediaOffer, ok := pool.Attributes[sa.Media]; ok {
			mediaOffers = append(mediaOffers, mediaOffer)
		}

		pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(false)
		pool.Attributes[sa.Clones] = sa.NewBoolOffer(false)
		pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes[sa.ProvisioningType] = sa.NewStringOffer(sa.Thick)

		pool.InternalAttributes[Size] = d.Config.Size
		pool.InternalAttributes[Region] = d.Config.Region
		pool.InternalAttributes[Zone] = d.Config.Zone

		d.physicalPools[pool.Name] = pool
	}

	// Define virtual pools
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

		pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(false)
		pool.Attributes[sa.Clones] = sa.NewBoolOffer(false)
		pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes[sa.ProvisioningType] = sa.NewStringOffer(sa.Thick)
		pool.Attributes[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)

		if region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(region)
		}
		if zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(zone)
		}
		if len(mediaOffers) > 0 {
			pool.Attributes[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
			pool.InternalAttributes[sa.Media] = pool.Attributes[sa.Media].ToString()
		}

		pool.InternalAttributes[Size] = size
		pool.InternalAttributes[Region] = region
		pool.InternalAttributes[Zone] = zone

		d.virtualPools[pool.Name] = pool
	}

	return nil
}

// Validate the driver configuration
func (d *SANStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	// Make sure the essential information was specified in the config
	if d.Config.WebProxyHostname == "" {
		return errors.New("WebProxyHostname is empty! You must specify the host/IP for the Web Services Proxy")
	}
	if d.Config.ControllerA == "" || d.Config.ControllerB == "" {
		return errors.New("ControllerA or ControllerB are empty! You must specify the host/IP for the " +
			"E-Series storage array. If it is a simplex array just specify the same host/IP twice")
	}
	if d.Config.HostDataIP == "" {
		return errors.New("HostDataIP is empty! You need to specify at least one of the iSCSI interface " +
			"IP addresses that is connected to the E-Series array")
	}

	// Validate pool-level attributes
	allPools := make([]*storage.Pool, 0, len(d.physicalPools)+len(d.virtualPools))

	for _, pool := range d.physicalPools {
		allPools = append(allPools, pool)
	}
	for _, pool := range d.virtualPools {
		allPools = append(allPools, pool)
	}

	for _, pool := range allPools {

		// Validate default size
		if _, err := utils.ConvertSizeToBytes(pool.InternalAttributes[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", pool.Name, err)
		}

		// Validate media type
		if pool.InternalAttributes[Media] != "" {
			for _, mediaType := range strings.Split(pool.InternalAttributes[Media], ",") {
				switch mediaType {
				case sa.HDD, sa.SSD:
					break
				default:
					log.Errorf("invalid media type in pool %s: %s", pool.Name, mediaType)
				}
			}
		}
	}

	return nil
}

// Create is called by Docker to create a container volume. Besides the volume name, a few optional parameters such as size
// and disk media type may be provided in the opts map. If more than one pool on the storage controller can satisfy the request, the
// one with the most free space is selected.
func (d *SANStorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	var fstype string

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "SANStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	extantVolume, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if d.API.IsRefValid(extantVolume.VolumeRef) {
		return drivers.NewVolumeExistsError(name)
	}

	// Get candidate physical pools
	physicalPools, err := d.getPoolsForCreate(volConfig, storagePool, volAttributes)
	if err != nil {
		return err
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
		defaultSize, _ := utils.ConvertSizeToBytes(d.Config.Size)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Get options
	opts, err := d.GetVolumeOpts(volConfig, storagePool, volAttributes)
	if err != nil {
		return err
	}

	// Get media type, or default to "hdd" if not specified
	mediaType := utils.GetV(opts, "mediaType", "")

	fstype, err = drivers.CheckSupportedFilesystem(utils.GetV(opts, "fstype|fileSystemType", drivers.DefaultFileSystemType), name)
	if err != nil {
		return err
	}

	createErrors := make([]error, 0)

	for _, physicalPool := range physicalPools {

		poolName := physicalPool.Name

		// expect pool of size 1
		pools, err := d.API.GetVolumePools(mediaType, sizeBytes, poolName)
		if err != nil {
			errMessage := fmt.Sprintf("E-series pool %s not found", poolName)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		// get first element of the pools
		pool := pools[0]

		// Create the volume
		vol, err := d.API.CreateVolume(name, pool.VolumeGroupRef, sizeBytes, mediaType, fstype)
		if err != nil {
			errMessage := fmt.Sprintf("E-series pool %s could not create volume %s: %v", poolName, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		log.WithFields(log.Fields{
			"Name":          name,
			"Size":          sizeBytes,
			"MediaType":     mediaType,
			"RequestedPool": storagePool.Name,
			"PhysicalPool":  poolName,
			"VolumeRef":     vol.VolumeRef,
		}).Debug("Create succeeded.")

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors)
}

// getPoolsForCreate returns candidate storage pools for creating volumes
func (d *SANStorageDriver) getPoolsForCreate(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) ([]*storage.Pool, error) {

	// If a physical pool was requested, just use it
	if _, ok := d.physicalPools[storagePool.Name]; ok {
		return []*storage.Pool{storagePool}, nil
	}

	// If a virtual pool was requested, find a physical pool to satisfy it
	if _, ok := d.virtualPools[storagePool.Name]; !ok {
		return nil, fmt.Errorf("could not find pool %s", storagePool.Name)
	}

	// Make a storage class from the volume attributes to simplify pool matching
	attributesCopy := make(map[string]sa.Request)
	for k, v := range volAttributes {
		attributesCopy[k] = v
	}
	delete(attributesCopy, sa.Selector)
	storageClass := sc.NewFromAttributes(attributesCopy)

	// Find matching pools
	candidatePools := make([]*storage.Pool, 0)

	for _, pool := range d.physicalPools {
		if storageClass.Matches(pool) {
			candidatePools = append(candidatePools, pool)
		}
	}

	if len(candidatePools) == 0 {
		err := errors.New("backend has no physical pools that can satisfy request")
		return nil, drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err})
	}

	// Shuffle physical pools
	rand.Shuffle(len(candidatePools), func(i, j int) {
		candidatePools[i], candidatePools[j] = candidatePools[j], candidatePools[i]
	})

	return candidatePools, nil
}

// Destroy is called by Docker to delete a container volume.
func (d *SANStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	vol, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", name, err)
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = d.getISCSITargetInfo()
		if err != nil {
			log.WithField("error", err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunID = -1
		for _, mapping := range vol.Mappings {
			lunID = mapping.LunNumber
		}
		if lunID >= 0 {
			// Inform the host about the device removal
			utils.PrepareDeviceForRemoval(lunID, iSCSINodeName)
		}
	}

	if d.API.IsRefValid(vol.VolumeRef) {

		// Destroy volume on storage array
		err = d.API.DeleteVolume(vol)
		if err != nil {
			return fmt.Errorf("could not destroy volume %s: %v", name, err)
		}

	} else {

		// If volume was deleted on this storage for any reason, don't fail it here.
		log.WithField("Name", name).Warn("Could not find volume on array. Allowing deletion to proceed.")
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(name string, publishInfo *utils.VolumePublishInfo) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	// Get the volume
	vol, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", name, err)
	}
	if !d.API.IsRefValid(vol.VolumeRef) {
		return fmt.Errorf("could not find volume %s", name)
	}

	// Get the Target IQN
	targetIQN, err := d.API.GetTargetIQN()
	if err != nil {
		return fmt.Errorf("could not get target IQN from array: %v", err)
	}

	// Get the fstype
	fstype := ""
	for _, tag := range vol.VolumeTags {
		if tag.Key == "fstype" {
			fstype = tag.Value
			log.WithFields(log.Fields{"LUN": name, "fstype": fstype}).Debug("Found LUN fstype.")
			break
		}
	}
	if fstype == "" {
		fstype = drivers.DefaultFileSystemType
		log.WithFields(log.Fields{"LUN": name, "fstype": fstype}).Warn("LUN fstype not found, using default.")
	}

	var iqn string
	var hostname string
	var mapping api.LUNMapping

	if publishInfo.Localhost {

		// Lookup local host IQNs
		iqns, err := utils.GetInitiatorIqns()
		if err != nil {
			return fmt.Errorf("error determining host initiator IQN: %v", err)
		} else if len(iqns) == 0 {
			return errors.New("could not determine host initiator IQN")
		}
		iqn = iqns[0]

		// Map the volume to the local host
		mapping, err = d.MapVolumeToLocalHost(vol)
		if err != nil {
			return fmt.Errorf("could not map volume %s: %v", name, err)
		}

	} else {

		// Host IQN must have been passed in
		if len(publishInfo.HostIQN) == 0 {
			return errors.New("host initiator IQN not specified")
		}
		iqn = publishInfo.HostIQN[0]
		hostname = publishInfo.HostName

		// Get the host group
		hostGroup, err := d.API.EnsureHostGroup()
		if err != nil {
			return fmt.Errorf("could not get host group: %v", err)
		}

		// See if there is already a host for the specified IQN
		host, err := d.API.GetHostForIQN(iqn)
		if err != nil {
			return fmt.Errorf("could not get host for IQN %s: %v", iqn, err)
		}

		// Create the host if necessary
		if host.HostRef == "" {
			host, err = d.API.CreateHost(hostname, iqn, d.Config.HostType, hostGroup)
			if err != nil {
				return fmt.Errorf("could not create host for IQN %s: %v", iqn, err)
			}
		}

		// If we got a host, make sure it's in the right group
		if host.HostRef != "" && host.ClusterRef != hostGroup.ClusterRef {
			return fmt.Errorf("found for IQN %s, but it is in host group %s: %v", iqn, d.Config.AccessGroup, err)
		}

		// Map the volume directly to the Host Group
		mapHost := api.HostEx{
			HostRef:    api.NullRef,
			ClusterRef: hostGroup.ClusterRef,
		}
		mapping, err = d.API.MapVolume(vol, mapHost)
		if err != nil {
			return fmt.Errorf("could not map volume %s to Host Group %s: %v", name, hostGroup.Label, err)
		}
	}

	// Add fields needed by Attach
	publishInfo.IscsiLunNumber = int32(mapping.LunNumber)
	publishInfo.IscsiTargetPortal = d.Config.HostDataIP
	publishInfo.IscsiTargetIQN = targetIQN
	publishInfo.FilesystemType = fstype
	publishInfo.UseCHAP = false
	publishInfo.SharedTarget = true

	return nil
}

func (d *SANStorageDriver) getISCSITargetInfo() (iSCSINodeName string, iSCSIInterfaces []string, returnError error) {

	targetSettings, err := d.API.GetTargetSettings()
	if err != nil {
		returnError = fmt.Errorf("could not get iSCSI target info: %v", err)
		return
	}
	iSCSINodeName = targetSettings.NodeName.IscsiNodeName
	for _, portal := range targetSettings.Portals {
		if portal.IPAddress.AddressType == "ipv4" {
			iSCSIInterface := fmt.Sprintf("%s:%d", portal.IPAddress.Ipv4Address, portal.TCPListenPort)
			iSCSIInterfaces = append(iSCSIInterfaces, iSCSIInterface)
		}
	}
	if len(iSCSIInterfaces) == 0 {
		returnError = errors.New("target has no active IPv4 iSCSI interfaces")
		return
	}

	return
}

// CreateHostForLocalHost ensures a Host definition corresponding to the local host exists on the array,
// defining a Host & HostGroup if not.
func (d *SANStorageDriver) CreateHostForLocalHost() (api.HostEx, error) {

	// Get the IQN for this host
	iqns, err := utils.GetInitiatorIqns()
	if err != nil {
		return api.HostEx{}, fmt.Errorf("could not determine host initiator IQNs: %v", err)
	}
	if len(iqns) == 0 {
		return api.HostEx{}, errors.New("could not determine host initiator IQNs")
	}
	iqn := iqns[0]

	// Ensure we have an E-series host to which to map the volume
	host, err := d.API.EnsureHostForIQN(iqn)
	if err != nil {
		return api.HostEx{}, fmt.Errorf("could not define array host for IQN %s: %v", iqn, err)
	}

	return host, nil
}

// MapVolumeToLocalHost gets the iSCSI identity of the local host, ensures a corresponding Host definition exists on the array
// (defining a Host & HostGroup if not), maps the specified volume to the host/group (if it isn't already), and returns the mapping info.
func (d *SANStorageDriver) MapVolumeToLocalHost(volume api.VolumeEx) (api.LUNMapping, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "MapVolumeToLocalHost",
			"Type":   "SANStorageDriver",
			"volume": volume.Label,
		}
		log.WithFields(fields).Debug(">>>> MapVolumeToLocalHost")
		defer log.WithFields(fields).Debug("<<<< MapVolumeToLocalHost")
	}

	// Ensure we have a host to map the volume to
	host, err := d.CreateHostForLocalHost()
	if err != nil {
		return api.LUNMapping{}, fmt.Errorf("could not map volume %s to host: %v", volume.Label, err)
	}

	// Map the volume
	mapping, err := d.API.MapVolume(volume, host)
	if err != nil {
		return api.LUNMapping{}, fmt.Errorf("could not map volume %s to host %s: %v", volume.Label, host.Label, err)
	}

	return mapping, nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *SANStorageDriver) GetSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshot")
		defer log.WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return nil, drivers.NewSnapshotsNotSupportedError(d.Name())
}

// SnapshotList returns the list of snapshots associated with the specified volume. The E-series volume
// plugin does not support snapshots, so this method always returns an empty array.
func (d *SANStorageDriver) GetSnapshots(volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "SANStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshots")
		defer log.WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return make([]*storage.Snapshot, 0), nil
}

// CreateSnapshot creates a snapshot for the given volume. The E-series volume plugin
// does not support cloning or snapshots, so this method always returns an error.
func (d *SANStorageDriver) CreateSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> CreateSnapshot")
		defer log.WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return nil, drivers.NewSnapshotsNotSupportedError(d.Name())
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer log.WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return drivers.NewSnapshotsNotSupportedError(d.Name())
}

// DeleteSnapshot deletes a volume snapshot.
func (d *SANStorageDriver) DeleteSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer log.WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return drivers.NewSnapshotsNotSupportedError(d.Name())
}

// CreateClone creates a new volume from the named volume, either by direct clone or from the named snapshot. The E-series volume plugin
// does not support cloning or snapshots, so this method always returns an error.
func (d *SANStorageDriver) CreateClone(volConfig *storage.VolumeConfig, storagePool *storage.Pool) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "SANStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	return fmt.Errorf("cloning is not supported by backend type %s", d.Name())
}

func (d *SANStorageDriver) Import(volConfig *storage.VolumeConfig, originalName string) error {
	return errors.New("import is not implemented")
}

func (d *SANStorageDriver) Rename(name string, newName string) error {
	return errors.New("rename is not implemented")
}

// Get test for the existence of a volume
func (d *SANStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Get",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.getVolume(name)
	if err != nil {
		return err
	}

	return nil
}

func (d *SANStorageDriver) getVolume(name string) (api.VolumeEx, error) {

	vol, err := d.API.GetVolume(name)
	if err != nil {
		return vol, fmt.Errorf("could not find volume %s: %v", name, err)
	} else if !d.API.IsRefValid(vol.VolumeRef) {
		return vol, fmt.Errorf("could not find volume %s", name)
	}
	log.WithField("volume", vol).Debug("Found volume.")

	return vol, nil
}

// GetStorageBackendSpecs retrieve storage capabilities and register pools with specified backend.
func (d *SANStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	backend.Name = d.backendName()

	virtual := len(d.virtualPools) > 0

	for _, pool := range d.physicalPools {
		pool.Backend = backend
		if !virtual {
			backend.AddStoragePool(pool)
		}
	}

	for _, pool := range d.virtualPools {
		pool.Backend = backend
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

func (d *SANStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
}

func (d *SANStorageDriver) GetInternalVolumeName(name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// E-series has a 30-character limitation on volume names, so no combination
		// of the usual namespace, PVC name, and PVC UID characters is likely to
		// fit, nor is some Base64 encoding of the same. And unfortunately, the PVC
		// UID is not persisted past the highest levels of Trident. So we borrow a
		// page from the E-series OpenStack driver and return a Base64-encoded form
		// of a new random (version 4) UUID.
		uuid4string := uuid.New().String()
		b64string, err := d.uuidToBase64(uuid4string)
		if err != nil {
			// This is unlikely, but if the UUID encoding fails, just return the original string (capped to 30 chars)
			if len(name) > 30 {
				return name[0:30]
			}
			return name
		}

		log.WithFields(log.Fields{
			"Name":   name,
			"UUID":   uuid4string,
			"Base64": b64string,
		}).Debug("EseriesStorageDriver#GetInternalVolumeName : Created Base64 UUID for E-series volume name.")

		return b64string
	}
}

func (d *SANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) (map[string]string, error) {

	opts := make(map[string]string)

	// Include the pool so that Trident's pool selection is honored by nDVP
	if pool != nil {
		opts["pool"] = pool.Name
	}

	// Include mediaType request if present
	if mediaTypeReq, ok := requests[sa.Media]; ok {
		if mediaType, ok := mediaTypeReq.Value().(string); ok {
			if mediaType == sa.HDD {
				opts["mediaType"] = "hdd"
			} else if mediaType == sa.SSD {
				opts["mediaType"] = "ssd"
			} else {
				log.WithFields(log.Fields{
					"provisioner":      "E-series",
					"method":           "GetVolumeOpts",
					"provisioningType": mediaTypeReq.Value(),
				}).Warnf("Expected 'ssd' or 'hdd' for %s; ignoring.", sa.Media)
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner":      "E-series",
				"method":           "GetVolumeOpts",
				"provisioningType": mediaTypeReq.Value(),
			}).Warnf("Expected string for %s; ignoring.", sa.Media)
		}
	}

	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}

	log.WithFields(log.Fields{
		"volConfig": volConfig,
		"pool":      pool,
		"requests":  requests,
		"opts":      opts,
	}).Debug("EseriesStorageDriver#GetVolumeOpts")

	return opts, nil
}

func (d *SANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateFollowup",
			"Type":         "SANStorageDriver",
			"name":         volConfig.Name,
			"internalName": volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> CreateFollowup")
		defer log.WithFields(fields).Debug("<<<< CreateFollowup")
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		log.Debug("No follow-up create actions for Docker.")
		return nil
	}

	// Get the volume
	name := volConfig.InternalName
	volume, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", name, err)
	}
	if !d.API.IsRefValid(volume.VolumeRef) {
		return fmt.Errorf("could not find volume %s", name)
	}

	// Get the Target IQN
	targetIQN, err := d.API.GetTargetIQN()
	if err != nil {
		return fmt.Errorf("could not get target IQN from array: %v", err)
	}

	// Get the Trident Host Group
	hostGroup, err := d.API.GetHostGroup(d.Config.AccessGroup)
	if err != nil {
		return fmt.Errorf("could not get Host Group %s from array: %v", d.Config.AccessGroup, err)
	}

	// Map the volume directly to the Host Group
	host := api.HostEx{
		HostRef:    api.NullRef,
		ClusterRef: hostGroup.ClusterRef,
	}
	mapping, err := d.API.MapVolume(volume, host)
	if err != nil {
		return fmt.Errorf("could not map volume %s to Host Group %s: %v", name, hostGroup.Label, err)
	}

	volConfig.AccessInfo.IscsiTargetPortal = d.Config.HostDataIP
	volConfig.AccessInfo.IscsiTargetIQN = targetIQN
	volConfig.AccessInfo.IscsiLunNumber = int32(mapping.LunNumber)

	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"hostGroup":       hostGroup.Label,
	}).Debug("Mapped E-series LUN.")

	return nil
}

func (d *SANStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	log.Debugln("EseriesStorageDriver:StoreConfig")

	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.EseriesConfig = &d.Config
}

func (d *SANStorageDriver) GetExternalConfig() interface{} {
	log.Debugln("EseriesStorageDriver:GetExternalConfig")

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.ESeriesStorageDriverConfig
	drivers.Clone(d.Config, &cloneConfig)
	cloneConfig.Username = ""      // redact the username
	cloneConfig.Password = ""      // redact the password
	cloneConfig.PasswordArray = "" // redact the password
	return cloneConfig
}

func (d *SANStorageDriver) uuidToBase64(UUID string) (string, error) {

	// Strip out hyphens
	UUID = strings.Replace(UUID, "-", "", -1)

	// Convert hex chars to binary
	var bytes [16]byte
	_, err := hex.Decode(bytes[:], []byte(UUID))
	if err != nil {
		return "", err
	}

	// Convert binary to Base64
	encoded := base64.RawURLEncoding.EncodeToString(bytes[:])

	return encoded, nil
}

func (d *SANStorageDriver) base64ToUUID(b64 string) (string, error) {

	// Convert Base64 to binary
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("error decoding Base64 string %s", b64)
	}

	// Convert binary to hex chars
	UUID := hex.EncodeToString(decoded[:])

	// Add hyphens
	UUID = strings.Join([]string{UUID[:8], UUID[8:12], UUID[12:16], UUID[16:20], UUID[20:]}, "-")

	return UUID, nil
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.GetVolume(name)
	if err != nil {
		return nil, err
	}
	if volumeAttrs.Label == "" {
		return nil, fmt.Errorf("volume %s not found", name)
	}

	return d.getVolumeExternal(&volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *SANStorageDriver) GetVolumeExternalWrappers(
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
	reposRegex, _ := regexp.Compile("^repos_\\d{4}$")

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range volumes {

		// Filter out internal volumes
		if reposRegex.MatchString(volume.Label) {
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.Label, prefix) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&volume), Error: nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANStorageDriver) getVolumeExternal(
	volumeAttrs *api.VolumeEx) *storage.VolumeExternal {

	internalName := volumeAttrs.Label
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            volumeAttrs.VolumeSize,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *SANStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.HostDataIP != dOrig.Config.HostDataIP {
		bitmap.Add(storage.VolumeAccessInfoChange)
	}

	if d.Config.Password != dOrig.Config.Password {
		bitmap.Add(storage.PasswordChange)
	}

	if d.Config.Username != dOrig.Config.Username {
		bitmap.Add(storage.UsernameChange)
	}

	return bitmap
}

// Resize expands the volume size. This method relies on the desired state model of Kubernetes
// and will not work with Docker.
func (d *SANStorageDriver) Resize(volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	vol, err := d.getVolume(name)
	if err != nil {
		return err
	}

	// Check to see if a volume expand operation is already being processed.
	// If true then return the error, to K8S, which indicates that the volume resize is in progress.
	// If no errors exist continue to attempt to resize the volume.
	isResizing, err := d.API.ResizingVolume(vol)
	if isResizing || (err != nil) {
		return err
	}

	volSizeBytes, err := strconv.ParseUint(vol.VolumeSize, 10, 64)
	if err != nil {
		return fmt.Errorf("error occurred when checking volume size")
	}

	sameSize, err := utils.VolumeSizeWithinTolerance(int64(sizeBytes), int64(volSizeBytes), tridentconfig.SANResizeDelta)
	if err != nil {
		return err
	}

	if sameSize {
		log.WithFields(log.Fields{
			"requestedSize":     sizeBytes,
			"currentVolumeSize": volSizeBytes,
			"name":              name,
			"delta":             tridentconfig.SANResizeDelta,
		}).Info("Requested size and current volume size are within the delta and therefore considered the same size for SAN resize operations.")
		return nil
	}

	if sizeBytes < volSizeBytes {

		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volSizeBytes)
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	d.API.ResizeVolume(vol, sizeBytes)

	// Check to see if a volume expand operation is still being processed.
	// If true then return the error, to K8S, which indicates that the volume resize is in progress.
	// If no errors exist then return nil as resize succeeded.
	isResizing, err = d.API.ResizingVolume(vol)
	if isResizing || (err != nil) {
		return err
	}

	// Update volSizeBytes to return new volume size
	vol, err = d.getVolume(name)
	volSizeBytes, err = strconv.ParseUint(vol.VolumeSize, 10, 64)
	if err != nil {
		return fmt.Errorf("error occurred when checking final volume size")
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}
