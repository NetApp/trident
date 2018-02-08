// Copyright 2016 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/pborman/uuid"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries/api"
	"github.com/netapp/trident/utils"
)

const DefaultHostType = "linux_dm_mp"
const EseriesMinimumVolumeSizeBytes = 1048576 // 1 MiB

// ESeriesStorageDriver is for storage provisioning via the Web Services Proxy RESTful interface that communicates
// with E-Series controllers via the SYMbol API.
type ESeriesStorageDriver struct {
	initialized bool
	Config      drivers.ESeriesStorageDriverConfig
	API         *api.APIClient
}

type ESeriesStorageDriverConfigExternal struct {
	*drivers.CommonStorageDriverConfigExternal
	Username    string
	ControllerA string
	ControllerB string
	HostDataIP  string
}

func (d *ESeriesStorageDriver) Name() string {
	return drivers.EseriesIscsiStorageDriverName
}

func (d *ESeriesStorageDriver) Protocol() string {
	return "iscsi"
}

// Initialize from the provided config
func (d *ESeriesStorageDriver) Initialize(
	context trident.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	// Trace logging hasn't been set up yet, so always do it here
	fields := log.Fields{
		"Method": "Initialize",
		"Type":   "ESeriesStorageDriver",
	}
	log.WithFields(fields).Debug(">>>> Initialize")
	defer log.WithFields(fields).Debug("<<<< Initialize")

	commonConfig.DriverContext = context

	config := &drivers.ESeriesStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// Decode configJSON into ESeriesStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return fmt.Errorf("Could not decode JSON configuration. %v", err)
	}

	// Apply config defaults
	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(context)
		config.StoragePrefix = &prefix
	}
	if config.AccessGroup == "" {
		config.AccessGroup = drivers.GetDefaultIgroupName(context)
	}
	if config.HostType == "" {
		config.HostType = DefaultHostType
	}
	if config.PoolNameSearchPattern == "" {
		config.PoolNameSearchPattern = ".+"
	}

	// Fix poorly-chosen config key
	if config.HostData_IP != "" && config.HostDataIP == "" {
		config.HostDataIP = config.HostData_IP
	}

	log.WithFields(log.Fields{
		"Version":           config.Version,
		"StorageDriverName": config.StorageDriverName,
		"DebugTraceFlags":   config.DebugTraceFlags,
		"DisableDelete":     config.DisableDelete,
		"StoragePrefix":     *config.StoragePrefix,
	}).Debug("Reparsed into ESeriesStorageDriverConfig")

	d.Config = *config

	// Ensure the config is valid
	err = d.validate()
	if err != nil {
		return fmt.Errorf("Could not validate ESeriesStorageDriver config. %v", err)
	}

	telemetry := make(map[string]string)
	telemetry["version"] = trident.OrchestratorVersion.ShortString()
	telemetry["platform"] = trident.OrchestratorTelemetry.Platform
	telemetry["platformVersion"] = trident.OrchestratorTelemetry.PlatformVersion
	telemetry["plugin"] = d.Name()
	telemetry["storagePrefix"] = *d.Config.StoragePrefix

	d.API = api.NewAPIClient(api.APIClientConfig{
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

	if context == trident.ContextDocker {
		// Make sure this host is logged into the E-series iSCSI target
		err = utils.EnsureIscsiSession(d.Config.HostDataIP)
		if err != nil {
			return fmt.Errorf("Could not establish iSCSI session. %v", err)
		}
	}

	// Connect to web services proxy
	_, err = d.API.Connect()
	if err != nil {
		return fmt.Errorf("Could not connect to Web Services Proxy. %v", err)
	}

	// Log controller serial numbers
	d.Config.SerialNumbers, err = d.API.ListNodeSerialNumbers()
	if err != nil {
		log.Warnf("Could not determine controller serial numbers. %v", err)
	} else {
		log.WithFields(log.Fields{
			"serialNumbers": strings.Join(d.Config.SerialNumbers, ","),
		}).Info("Controller serial numbers.")
	}

	d.initialized = true
	return nil
}

func (d *ESeriesStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *ESeriesStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "ESeriesStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// Validate the driver configuration
func (d *ESeriesStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "ESeriesStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	// Make sure the essential information was specified in the config
	if d.Config.WebProxyHostname == "" {
		return errors.New("WebProxyHostname is empty! You must specify the host/IP for the Web Services Proxy.")
	}
	if d.Config.ControllerA == "" || d.Config.ControllerB == "" {
		return errors.New("ControllerA or ControllerB are empty! You must specify the host/IP for the " +
			"E-Series storage array. If it is a simplex array just specify the same host/IP twice.")
	}
	if d.Config.HostDataIP == "" {
		return errors.New("HostDataIP is empty! You need to specify at least one of the iSCSI interface " +
			"IP addresses that is connected to the E-Series array.")
	}

	return nil
}

// Create is called by Docker to create a container volume. Besides the volume name, a few optional parameters such as size
// and disk media type may be provided in the opts map. If more than one pool on the storage controller can satisfy the request, the
// one with the most free space is selected.
func (d *ESeriesStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "ESeriesStorageDriver",
			"name":   name,
			"opts":   opts,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	if sizeBytes < EseriesMinimumVolumeSizeBytes {
		return fmt.Errorf("Requested volume size (%d bytes) is too small.  The minimum volume size is %d bytes.",
			sizeBytes, EseriesMinimumVolumeSizeBytes)
	}

	// Get media type, or default to "hdd" if not specified
	mediaType := utils.GetV(opts, "mediaType", "")

	// Check for a supported file system type
	fstype := strings.ToLower(utils.GetV(opts, "fstype|fileSystemType", "ext4"))
	switch fstype {
	case "xfs", "ext3", "ext4":
		log.WithFields(log.Fields{"fileSystemType": fstype, "name": name}).Debug("Filesystem format.")
	default:
		return fmt.Errorf("Unsupported fileSystemType option: %s.", fstype)
	}

	// Get pool name, or default to all pools if not specified
	poolName := utils.GetV(opts, "pool", "")

	pools, err := d.API.GetVolumePools(mediaType, sizeBytes, poolName)
	if err != nil {
		return fmt.Errorf("Create failed. %v", err)
	} else if len(pools) == 0 {
		return errors.New("Create failed. No storage pools matched specified parameters.")
	}

	log.Debugf("Got pools for create: %v", pools)

	// Pick the pool with the largest free space
	sort.Sort(sort.Reverse(api.ByFreeSpace(pools)))
	pool := pools[0]

	// Create the volume
	vol, err := d.API.CreateVolume(name, pool.VolumeGroupRef, sizeBytes, mediaType, fstype)
	if err != nil {
		return fmt.Errorf("Could not create volume %s. %v", name, err)
	}

	log.WithFields(log.Fields{
		"Name":      name,
		"Size":      sizeBytes,
		"MediaType": mediaType,
		"VolumeRef": vol.VolumeRef,
		"Pool":      pool.Label,
	}).Debug("Create succeeded.")

	return nil
}

// Create is called by Docker to delete a container volume.
func (d *ESeriesStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "ESeriesStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	vol, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find volume %s. %v", name, err)
	}

	if d.API.IsRefValid(vol.VolumeRef) {

		// Destroy volume on storage array
		err = d.API.DeleteVolume(vol)
		if err != nil {
			return fmt.Errorf("Could not destroy volume %s. %v", name, err)
		}

	} else {

		// If volume was deleted on this storage for any reason, don't fail it here.
		log.WithField("Name", name).Warn("Could not find volume on array. Allowing deletion to proceed.")
	}

	// Perform rediscovery to remove the deleted LUN
	if d.Config.DriverContext == trident.ContextDocker {
		utils.MultipathFlush() // flush unused paths
		utils.IscsiRescan(true)
	}

	return nil
}

// Attach is called by Docker when attaching a container volume to a container. This method is expected to map the volume
// to the local host, discover it on the SCSI bus, format it with a filesystem, and mount it at the specified mount point.
// This method has an opts parameter, but no options are presently handled by this method.
func (d *ESeriesStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Attach",
			"Type":       "ESeriesStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
			"opts":       opts,
		}
		log.WithFields(fields).Debug(">>>> Attach")
		defer log.WithFields(fields).Debug("<<<< Attach")
	}

	// Get the volume
	vol, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find volume %s. %v", name, err)
	}
	if !d.API.IsRefValid(vol.VolumeRef) {
		return fmt.Errorf("Could not find volume %s.", name)
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
		fstype = "ext4"
		log.WithFields(log.Fields{"LUN": name, "fstype": fstype}).Warn("LUN fstype not found, using default.")
	}

	// Map the volume to the local host
	mapping, err := d.MapVolumeToLocalHost(vol)
	if err != nil {
		return fmt.Errorf("Could not map volume %s. %v", name, err)
	}

	// Rescan the SCSI bus to ensure the host sees the LUN
	err = utils.IscsiRescan(false)
	if err != nil {
		return fmt.Errorf("Could not rescan the SCSI bus. %v", err)
	}

	// Get the SCSI device information
	deviceInfo, err := utils.GetDeviceInfoForLuns()
	if err != nil {
		return fmt.Errorf("Could not get SCSI device information. %v", err)
	}

	// Get the iSCSI session information
	sessionInfo, err := utils.GetIscsiSessionInfo()
	if err != nil {
		return fmt.Errorf("Could not get iSCSI session information. %v", err)
	}

	sessionInfoToUse := utils.IscsiSessionInfo{}
	for i, e := range sessionInfo {
		if e.PortalIP == d.Config.HostDataIP {
			sessionInfoToUse = sessionInfo[i]
			break
		}
	}
	if sessionInfoToUse.TargetName == "" {
		return errors.New("Could not get iSCSI session information.")
	}

	deviceToUse := d.findDevice(mapping.LunNumber, sessionInfoToUse, deviceInfo)
	if deviceToUse.Device == "" {
		return fmt.Errorf("Could not determine device to use for volume %s.", vol.Label)
	}

	deviceRef := deviceToUse.Device
	if deviceToUse.MultipathDevice != "" {
		deviceRef = deviceToUse.MultipathDevice
	}

	// Put a filesystem on the volume if there isn't one already there
	if deviceToUse.Filesystem == "" {
		log.WithFields(log.Fields{"LUN": name, "fstype": fstype}).Debug("Formatting LUN.")
		err := utils.FormatVolume(deviceRef, fstype)
		if err != nil {
			return fmt.Errorf("Could not format volume %s, device %v. %v", name, deviceToUse, err)
		}
	} else if deviceToUse.Filesystem != fstype {
		log.WithFields(log.Fields{
			"volume":          name,
			"existingFstype":  deviceToUse.Filesystem,
			"requestedFstype": fstype,
		}).Warn("LUN already formatted with a different file system type.")
	} else {
		log.WithFields(log.Fields{"LUN": name, "fstype": deviceToUse.Filesystem}).Debug("LUN already formatted.")
	}

	// Mount the volume
	err = utils.Mount(deviceRef, mountpoint)
	if err != nil {
		return fmt.Errorf("Could not mount volume %s, device %v at mount point %s. %v", name, deviceToUse, mountpoint, err)
	}

	return nil
}

// MapVolumeToLocalHost gets the iSCSI identity of the local host, ensures a corresponding Host definition exists on the array
// (defining a Host & HostGroup if not), maps the specified volume to the host/group (if it isn't already), and returns the mapping info.
func (d *ESeriesStorageDriver) MapVolumeToLocalHost(volume api.VolumeEx) (api.LUNMapping, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "MapVolumeToLocalHost",
			"Type":   "ESeriesStorageDriver",
			"volume": volume.Label,
		}
		log.WithFields(fields).Debug(">>>> MapVolumeToLocalHost")
		defer log.WithFields(fields).Debug("<<<< MapVolumeToLocalHost")
	}

	// Get the IQN for this host
	iqns, err := utils.GetInitiatorIqns()
	if err != nil {
		return api.LUNMapping{}, fmt.Errorf("Could not determine host initiator IQNs. %v", err)
	}
	if len(iqns) == 0 {
		return api.LUNMapping{}, errors.New("Could not determine host initiator IQNs.")
	}
	iqn := iqns[0]

	// Ensure we have an E-series host to which to map the volume
	host, err := d.API.EnsureHostForIQN(iqn)
	if err != nil {
		return api.LUNMapping{}, fmt.Errorf("Could not define array host for IQN %s. %v", iqn, err)
	}

	// Map the volume
	mapping, err := d.API.MapVolume(volume, host)
	if err != nil {
		return api.LUNMapping{}, fmt.Errorf("Could not map volume %s to host %s. %v", volume.Label, host.Label, err)
	}

	return mapping, nil
}

// findDevice combs through a list of SCSI devices to find the one matching the specified LUN number and iSCSI session info. If no
// match is found, this method returns an empty structure, so the caller should check for empty values in the result.
func (d *ESeriesStorageDriver) findDevice(
	volumeLunNumber int, sessionInfo utils.IscsiSessionInfo, devices []utils.ScsiDeviceInfo) utils.ScsiDeviceInfo {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":          "findDevice",
			"Type":            "ESeriesStorageDriver",
			"volumeLunNumber": volumeLunNumber,
			"sessionInfo":     sessionInfo,
		}
		log.WithFields(fields).Debug(">>>> findDevice")
		defer log.WithFields(fields).Debug("<<<< findDevice")
	}

	// Look for the expected mapped LUN
	for i, device := range devices {

		log.WithFields(log.Fields{"i": i, "device": device}).Debug("Checking device.")

		// LUN number must match
		if device.LUN != strconv.Itoa(volumeLunNumber) {
			log.WithFields(log.Fields{"lunID": device.LUN}).Debug("Skipping device, LUN ID does not match.")
			continue
		}

		// Target IQN must match
		if !strings.HasPrefix(device.IQN, sessionInfo.TargetName) {
			log.WithFields(log.Fields{"IQN": device.IQN}).Debug("Skipping device, IQN does not match.")
			continue
		}

		log.WithFields(log.Fields{"Device": device}).Debug("Using device.")

		return device
	}
	return utils.ScsiDeviceInfo{}
}

// Attach is called by Docker when detaching a container volume from a container. This method merely
// unmounts the volume; it does not rescan the bus, unmap the volume, or undo any of the other actions
// taken by the Attach method.
func (d *ESeriesStorageDriver) Detach(name, mountpoint string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Detach",
			"Type":       "ESeriesStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
		}
		log.WithFields(fields).Debug(">>>> Detach")
		defer log.WithFields(fields).Debug("<<<< Detach")
	}

	cmd := fmt.Sprintf("umount %s", mountpoint)

	log.WithFields(log.Fields{"Command": cmd}).Debug("Unmounting volume")

	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.WithFields(log.Fields{"result": string(out)}).Debug("Unmount failed.")
		return fmt.Errorf("Could not unmount docker volume: %v mountpoint: %v error: %v", name, mountpoint, err)
	}

	return nil
}

// SnapshotList returns the list of snapshots associated with the named volume. The E-series volume plugin does not support snapshots,
// so this method always returns an empty array.
func (d *ESeriesStorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "SnapshotList",
			"Type":   "ESeriesStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> SnapshotList")
		defer log.WithFields(fields).Debug("<<<< SnapshotList")
	}

	return make([]storage.Snapshot, 0), nil
}

// CreateClone creates a new volume from the named volume, either by direct clone or from the named snapshot. The E-series volume plugin
// does not support cloning or snapshots, so this method always returns an error.
func (d *ESeriesStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "ESeriesStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	return errors.New("Cloning with E-Series is not supported.")
}

// Return the list of volumes associated with this tenant
func (d *ESeriesStorageDriver) List() ([]string, error) {
	prefix := *d.Config.StoragePrefix

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "List",
			"Type":   "ESeriesStorageDriver",
			"prefix": prefix,
		}
		log.WithFields(fields).Debug(">>>> List")
		defer log.WithFields(fields).Debug("<<<< List")
	}

	volumeNames, err := d.API.ListVolumes()
	if err != nil {
		return nil, fmt.Errorf("Could not get the list of volumes: %v", err)
	}

	// Filter out internal volumes
	filteredVolumeNames := make([]string, 0, len(volumeNames))
	reposRegex, _ := regexp.Compile("^repos_\\d{4}$")
	for _, name := range volumeNames {
		if !reposRegex.MatchString(name) {
			filteredVolumeNames = append(filteredVolumeNames, name)
		}
	}

	if len(prefix) == 0 {

		// No prefix, so just return the whole list
		log.WithField("Count", len(filteredVolumeNames)).Debug("Returning list of all volume names.")
		return filteredVolumeNames, nil

	} else {

		// Return only the volume names with the specified prefix
		prefixedVolumeNames := make([]string, 0, len(filteredVolumeNames))
		for _, name := range filteredVolumeNames {

			if !strings.HasPrefix(name, prefix) {
				continue
			}

			// The prefix shouldn't be visible to the user
			prefixedVolumeNames = append(prefixedVolumeNames, strings.TrimPrefix(name, prefix))
		}

		log.WithFields(log.Fields{
			"Count":  len(prefixedVolumeNames),
			"Prefix": prefix,
		}).Debug("Returning list of prefixed volume names.")
		return prefixedVolumeNames, nil
	}
}

// Test for the existence of a volume
func (d *ESeriesStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Get",
			"Type":   "ESeriesStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	vol, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find volume %s. %v", name, err)
	} else if !d.API.IsRefValid(vol.VolumeRef) {
		return fmt.Errorf("Could not find volume %s.", name)
	}
	log.WithField("volume", vol).Debug("Found volume.")

	return nil
}

// Retrieve storage capabilities and register pools with specified backend.
func (d *ESeriesStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "eseries_" + d.Config.HostDataIP

	// Get pools
	pools, err := d.API.GetVolumePools("", 0, "")
	if err != nil {
		return fmt.Errorf("Could not get storage pools from array. %v", err)
	}

	for _, pool := range pools {

		vc := storage.NewStoragePool(backend, pool.Label)
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())

		// E-series supports both "hdd" and "ssd" media types
		switch pool.DriveMediaType {
		case "hdd":
			vc.Attributes[sa.Media] = sa.NewStringOffer(sa.HDD)
		case "ssd":
			vc.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		}

		// No snapshots, clones or thin provisioning on E-series
		vc.Attributes[sa.Snapshots] = sa.NewBoolOffer(false)
		vc.Attributes[sa.Clones] = sa.NewBoolOffer(false)
		vc.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		vc.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thick")

		backend.AddStoragePool(vc)

		log.WithFields(log.Fields{
			"attributes": fmt.Sprintf("%+v", vc.Attributes),
			"pool":       pool.Label,
			"backend":    backend.Name,
		}).Debug("EseriesStorageDriver#GetStorageBackendSpecs : Added pool for E-series backend.")
	}

	return nil
}

func (d *ESeriesStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {

	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	// 2. Ensure no volume with the same name exists on that backend (unnecessary since
	// Step 1 always generates a new UUID-based name)

	return true
}

func (d *ESeriesStorageDriver) GetInternalVolumeName(name string) string {

	if trident.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// E-series has a 30-character limitation on volume names, so no combination
		// of the usual namespace, PVC name, and PVC UID characters is likely to
		// fit, nor is some Base64 encoding of the same. And unfortunately, the PVC
		// UID is not persisted past the highest levels of Trident. So we borrow a
		// page from the E-series OpenStack driver and return a Base64-encoded form
		// of a new random (version 4) UUID.
		uuid4string := uuid.New()
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

func (d *ESeriesStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
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

func (d *ESeriesStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	// Get the volume
	name := volConfig.InternalName
	volume, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find volume %s. %v", name, err)
	}
	if !d.API.IsRefValid(volume.VolumeRef) {
		return fmt.Errorf("Could not find volume %s.", name)
	}

	// Get the Target IQN
	targetIQN, err := d.API.GetTargetIQN()
	if err != nil {
		return fmt.Errorf("Could not get target IQN from array. %v", err)
	}

	// Get the Trident Host Group
	hostGroup, err := d.API.GetHostGroup(d.Config.AccessGroup)
	if err != nil {
		return fmt.Errorf("Could not get Host Group %s from array. %v", d.Config.AccessGroup, err)
	}

	// Map the volume directly to the Host Group
	host := api.HostEx{
		HostRef:    api.NullRef,
		ClusterRef: hostGroup.ClusterRef,
	}
	mapping, err := d.API.MapVolume(volume, host)
	if err != nil {
		return fmt.Errorf("Could not map volume %s to Host Group %s. %v", name, hostGroup.Label, err)
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
	}).Debug("EseriesStorageDriver#CreateFollowup : Successfully mapped E-series LUN.")

	return nil
}

func (d *ESeriesStorageDriver) GetProtocol() trident.Protocol {
	return trident.Block
}

func (d *ESeriesStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	log.Debugln("EseriesStorageDriver:StoreConfig")

	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.EseriesConfig = &d.Config
}

func (d *ESeriesStorageDriver) GetExternalConfig() interface{} {
	log.Debugln("EseriesStorageDriver:GetExternalConfig")

	return &ESeriesStorageDriverConfigExternal{
		CommonStorageDriverConfigExternal: drivers.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		Username:    d.Config.Username,
		ControllerA: d.Config.ControllerA,
		ControllerB: d.Config.ControllerB,
		HostDataIP:  d.Config.HostDataIP,
	}
}

func (d *ESeriesStorageDriver) uuidToBase64(UUID string) (string, error) {

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

func (d *ESeriesStorageDriver) base64ToUuid(b64 string) (string, error) {

	// Convert Base64 to binary
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("Error decoding Base64 string %s", b64)
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
func (d *ESeriesStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.GetVolume(name)
	if err != nil {
		return nil, err
	}
	if volumeAttrs.Label == "" {
		return nil, fmt.Errorf("Volume %s not found.", name)
	}

	poolAttrs, err := d.API.GetVolumePoolByRef(volumeAttrs.VolumeGroupRef)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&volumeAttrs, &poolAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *ESeriesStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.API.GetVolumes()
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Get all pools
	pools, err := d.API.GetVolumePools("", 0, "")
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Make a map of pools for faster correlation with volumes
	poolMap := make(map[string]api.VolumeGroupEx)
	for _, pool := range pools {
		poolMap[pool.VolumeGroupRef] = pool
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

		pool, ok := poolMap[volume.VolumeGroupRef]
		if !ok {
			log.WithField("volume", volume.Label).Warning("Pool not found for volume.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&volume, &pool), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *ESeriesStorageDriver) getVolumeExternal(
	volumeAttrs *api.VolumeEx, poolAttrs *api.VolumeGroupEx) *storage.VolumeExternal {

	internalName := volumeAttrs.Label
	name := internalName[len(*d.Config.StoragePrefix):]

	volumeConfig := &storage.VolumeConfig{
		Version:         trident.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            volumeAttrs.VolumeSize,
		Protocol:        trident.Block,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      trident.ReadWriteOnce,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   poolAttrs.Label,
	}
}
