// Copyright 2018 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

const (
	LSMirrorIdleTimeoutSecs      = 30
	MinimumVolumeSizeBytes       = 20971520 // 20 MiB
	HousekeepingStartupDelaySecs = 10
)

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin        string        `json:"plugin"`
	SVM           string        `json:"svm"`
	StoragePrefix string        `json:"storagePrefix"`
	Driver        StorageDriver `json:"-"`
	done          chan struct{} `json:"-"`
	ticker        *time.Ticker  `json:"-"`
}

type StorageDriver interface {
	GetConfig() *drivers.OntapStorageDriverConfig
	GetAPI() *api.Client
	GetTelemetry() *Telemetry
	Name() string
}

// InitializeOntapConfig parses the ONTAP config, mixing in the specified common config.
func InitializeOntapConfig(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) (*drivers.OntapStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapConfig", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> InitializeOntapConfig")
		defer log.WithFields(fields).Debug("<<<< InitializeOntapConfig")
	}

	commonConfig.DriverContext = context

	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into OntapStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration: %v", err)
	}

	return config, nil
}

func NewOntapTelemetry(d StorageDriver) *Telemetry {
	t := &Telemetry{
		Plugin:        d.Name(),
		SVM:           d.GetConfig().SVM,
		StoragePrefix: *d.GetConfig().StoragePrefix,
		Driver:        d,
		done:          make(chan struct{}),
	}

	usageHeartbeat := d.GetConfig().UsageHeartbeat
	heartbeatIntervalInHours := 24.0 // default to 24 hours
	if usageHeartbeat != "" {
		f, err := strconv.ParseFloat(usageHeartbeat, 64)
		if err != nil {
			log.WithField("interval", usageHeartbeat).Warnf("Invalid heartbeat interval. %v", err)
		} else {
			heartbeatIntervalInHours = f
		}
	}
	log.WithField("intervalHours", heartbeatIntervalInHours).Debug("Configured EMS heartbeat.")

	durationInHours := time.Millisecond * time.Duration(MSecPerHour*heartbeatIntervalInHours)
	if durationInHours > 0 {
		t.ticker = time.NewTicker(durationInHours)
	}
	return t
}

// Start starts the flow of ASUP messages for the driver
// These messages can be viewed via filer::> event log show -severity NOTICE.
func (t *Telemetry) Start() {
	go func() {
		time.Sleep(HousekeepingStartupDelaySecs * time.Second)
		EMSHeartbeat(t.Driver)
		for {
			select {
			case tick := <-t.ticker.C:
				log.WithFields(log.Fields{
					"tick":   tick,
					"driver": t.Driver.Name(),
				}).Debug("Sending EMS heartbeat.")
				EMSHeartbeat(t.Driver)
			case <-t.done:
				log.WithFields(log.Fields{
					"driver": t.Driver.Name(),
				}).Debugf("Shut down EMS logs for the driver.")
				return
			}
		}
	}()
}

func (t *Telemetry) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
	close(t.done)
}

// InitializeOntapDriver sets up the API client and performs all other initialization tasks
// that are common to all the ONTAP drivers.
func InitializeOntapDriver(config *drivers.OntapStorageDriverConfig) (*api.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapDriver", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> InitializeOntapDriver")
		defer log.WithFields(fields).Debug("<<<< InitializeOntapDriver")
	}

	// Splitting config.ManagementLIF with colon allows to provide managementLIF value as address:port format
	mgmtLIF := strings.Split(config.ManagementLIF, ":")[0]

	addressesFromHostname, err := net.LookupHost(mgmtLIF)
	if err != nil {
		log.WithField("ManagementLIF", mgmtLIF).Error("Host lookup failed for ManagementLIF. ", err)
		return nil, err
	}

	log.WithFields(log.Fields{
		"hostname":  mgmtLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from ManagementLIF lookup.")

	// Get the API client
	client, err := InitializeOntapAPI(config)
	if err != nil {
		return nil, fmt.Errorf("could not create Data ONTAP API client: %v", err)
	}

	// Make sure we're using a valid ONTAP version
	ontapi, err := client.SystemGetOntapiVersion()
	if err != nil {
		return nil, fmt.Errorf("could not determine Data ONTAP API version: %v", err)
	}
	if !client.SupportsFeature(api.MinimumONTAPIVersion) {
		return nil, errors.New("ONTAP 8.3 or later is required")
	}
	log.WithField("Ontapi", ontapi).Debug("ONTAP API version.")

	// Log cluster node serial numbers if we can get them
	config.SerialNumbers, err = client.NodeListSerialNumbers()
	if err != nil {
		log.Warnf("Could not determine controller serial numbers. %v", err)
	} else {
		log.WithFields(log.Fields{
			"serialNumbers": strings.Join(config.SerialNumbers, ","),
		}).Info("Controller serial numbers.")
	}

	// Load default config parameters
	err = PopulateConfigurationDefaults(config)
	if err != nil {
		return nil, fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	return client, nil
}

// InitializeOntapAPI returns an ontap.Client ZAPI client.  If the SVM isn't specified in the config
// file, this method attempts to derive the one to use.
func InitializeOntapAPI(config *drivers.OntapStorageDriverConfig) (*api.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapAPI", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> InitializeOntapAPI")
		defer log.WithFields(fields).Debug("<<<< InitializeOntapAPI")
	}

	client := api.NewClient(api.ClientConfig{
		ManagementLIF:   config.ManagementLIF,
		SVM:             config.SVM,
		Username:        config.Username,
		Password:        config.Password,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	if config.SVM != "" {

		vserverResponse, err := client.VserverGetRequest()
		if err = api.GetError(vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error reading SVM details: %v", err)
		}

		client.SVMUUID = string(vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid())

		log.WithField("SVM", config.SVM).Debug("Using specified SVM.")
		return client, nil
	}

	// Use VserverGetIterRequest to populate config.SVM if it wasn't specified and we can derive it
	vserverResponse, err := client.VserverGetIterRequest()
	if err = api.GetError(vserverResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating SVMs: %v", err)
	}

	if vserverResponse.Result.NumRecords() != 1 {
		return nil, errors.New("cannot derive SVM to use; please specify SVM in config file")
	}

	// Update everything to use our derived SVM
	config.SVM = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverName()
	svmUUID := string(vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].Uuid())

	client = api.NewClient(api.ClientConfig{
		ManagementLIF:   config.ManagementLIF,
		SVM:             config.SVM,
		Username:        config.Username,
		Password:        config.Password,
		DebugTraceFlags: config.DebugTraceFlags,
	})
	client.SVMUUID = svmUUID

	log.WithField("SVM", config.SVM).Debug("Using derived SVM.")
	return client, nil
}

// ValidateNASDriver contains the validation logic shared between ontap-nas and ontap-nas-economy.
func ValidateNASDriver(api *api.Client, config *drivers.OntapStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ValidateNASDriver", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> ValidateNASDriver")
		defer log.WithFields(fields).Debug("<<<< ValidateNASDriver")
	}

	dataLIFs, err := api.NetInterfaceGetDataLIFs("nfs")
	if err != nil {
		return err
	}

	if len(dataLIFs) == 0 {
		return fmt.Errorf("no NAS data LIFs found on SVM %s", config.SVM)
	} else {
		log.WithField("dataLIFs", dataLIFs).Debug("Found NAS LIFs.")
	}

	// If they didn't set a LIF to use in the config, we'll set it to the first nfs LIF we happen to find
	if config.DataLIF == "" {
		config.DataLIF = dataLIFs[0]
	} else {
		_, err := ValidateDataLIF(config.DataLIF, dataLIFs)
		if err != nil {
			return fmt.Errorf("data LIF validation failed: %v", err)
		}
	}

	return nil
}

func ValidateDataLIF(dataLIF string, dataLIFs []string) ([]string, error) {

	addressesFromHostname, err := net.LookupHost(dataLIF)
	if err != nil {
		log.Error("Host lookup failed. ", err)
		return nil, err
	}

	log.WithFields(log.Fields{
		"hostname":  dataLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from hostname lookup.")

	for _, hostNameAddress := range addressesFromHostname {
		foundValidLIFAddress := false

	loop:
		for _, lifAddress := range dataLIFs {
			if lifAddress == hostNameAddress {
				foundValidLIFAddress = true
				break loop
			}
		}
		if foundValidLIFAddress {
			log.WithField("hostNameAddress", hostNameAddress).Debug("Found matching Data LIF.")
		} else {
			log.WithField("hostNameAddress", hostNameAddress).Debug("Could not find matching Data LIF.")
			return nil, fmt.Errorf("could not find Data LIF for %s", hostNameAddress)
		}

	}

	return addressesFromHostname, nil
}

const DefaultSpaceReserve = "none"
const DefaultSnapshotPolicy = "none"
const DefaultSnapshotReserve = ""
const DefaultUnixPermissions = "---rwxrwxrwx"
const DefaultSnapshotDir = "false"
const DefaultExportPolicy = "default"
const DefaultSecurityStyle = "unix"
const DefaultNfsMountOptions = "-o nfsvers=3"
const DefaultSplitOnClone = "false"
const DefaultFileSystemType = "ext4"
const DefaultEncryption = "false"
const DefaultLimitAggregateUsage = ""
const DefaultLimitVolumeSize = ""

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func PopulateConfigurationDefaults(config *drivers.OntapStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "PopulateConfigurationDefaults", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> PopulateConfigurationDefaults")
		defer log.WithFields(fields).Debug("<<<< PopulateConfigurationDefaults")
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

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
	}

	if config.SpaceReserve == "" {
		config.SpaceReserve = DefaultSpaceReserve
	}

	if config.SnapshotPolicy == "" {
		config.SnapshotPolicy = DefaultSnapshotPolicy
	}

	if config.SnapshotReserve == "" {
		config.SnapshotReserve = DefaultSnapshotReserve
	}

	if config.UnixPermissions == "" {
		config.UnixPermissions = DefaultUnixPermissions
	}

	if config.SnapshotDir == "" {
		config.SnapshotDir = DefaultSnapshotDir
	}

	if config.ExportPolicy == "" {
		config.ExportPolicy = DefaultExportPolicy
	}

	if config.SecurityStyle == "" {
		config.SecurityStyle = DefaultSecurityStyle
	}

	if config.NfsMountOptions == "" {
		config.NfsMountOptions = DefaultNfsMountOptions
	}

	if config.SplitOnClone == "" {
		config.SplitOnClone = DefaultSplitOnClone
	} else {
		_, err := strconv.ParseBool(config.SplitOnClone)
		if err != nil {
			return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
		}
	}

	if config.FileSystemType == "" {
		config.FileSystemType = DefaultFileSystemType
	}

	if config.Encryption == "" {
		config.Encryption = DefaultEncryption
	}

	if config.LimitAggregateUsage == "" {
		config.LimitAggregateUsage = DefaultLimitAggregateUsage
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = DefaultLimitVolumeSize
	}

	log.WithFields(log.Fields{
		"StoragePrefix":       *config.StoragePrefix,
		"SpaceReserve":        config.SpaceReserve,
		"SnapshotPolicy":      config.SnapshotPolicy,
		"SnapshotReserve":     config.SnapshotReserve,
		"UnixPermissions":     config.UnixPermissions,
		"SnapshotDir":         config.SnapshotDir,
		"ExportPolicy":        config.ExportPolicy,
		"SecurityStyle":       config.SecurityStyle,
		"NfsMountOptions":     config.NfsMountOptions,
		"SplitOnClone":        config.SplitOnClone,
		"FileSystemType":      config.FileSystemType,
		"Encryption":          config.Encryption,
		"LimitAggregateUsage": config.LimitAggregateUsage,
		"LimitVolumeSize":     config.LimitVolumeSize,
		"Size":                config.Size,
	}).Debugf("Configuration defaults")

	return nil
}

// ValidateEncryptionAttribute returns true/false if encryption is being requested of a backend that
// supports NetApp Volume Encryption, and nil otherwise so that the ZAPIs may be sent without
// any reference to encryption.
func ValidateEncryptionAttribute(encryption string, client *api.Client) (*bool, error) {

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return nil, fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	if client.SupportsFeature(api.NetAppVolumeEncryption) {
		return &enableEncryption, nil
	} else {
		if enableEncryption {
			return nil, errors.New("encrypted volumes are not supported on this storage backend")
		} else {
			return nil, nil
		}
	}
}

func checkAggregateLimitsForFlexvol(
	flexvol string, requestedSizeInt uint64, config drivers.OntapStorageDriverConfig, client *api.Client,
) error {

	var aggregate, spaceReserve string

	volInfo, err := client.VolumeGet(flexvol)
	if err != nil {
		return err
	}
	if volInfo.VolumeIdAttributesPtr != nil {
		aggregate = volInfo.VolumeIdAttributesPtr.ContainingAggregateName()
	} else {
		return fmt.Errorf("aggregate info not available from Flexvol %s", flexvol)
	}
	if volInfo.VolumeSpaceAttributesPtr != nil {
		spaceReserve = volInfo.VolumeSpaceAttributesPtr.SpaceGuarantee()
	} else {
		return fmt.Errorf("spaceReserve info not available from Flexvol %s", flexvol)
	}

	return checkAggregateLimits(aggregate, spaceReserve, requestedSizeInt, config, client)
}

func checkAggregateLimits(
	aggregate, spaceReserve string, requestedSizeInt uint64,
	config drivers.OntapStorageDriverConfig, client *api.Client,
) error {

	requestedSize := float64(requestedSizeInt)

	limitAggregateUsage := config.LimitAggregateUsage
	limitAggregateUsage = strings.Replace(limitAggregateUsage, "%", "", -1) // strip off any %

	log.WithFields(log.Fields{
		"aggregate":           aggregate,
		"requestedSize":       requestedSize,
		"limitAggregateUsage": limitAggregateUsage,
	}).Debugf("Checking aggregate limits")

	if limitAggregateUsage == "" {
		log.Debugf("No limits specified")
		return nil
	}

	if aggregate == "" {
		return errors.New("aggregate not provided, cannot check aggregate provisioning limits")
	}

	// lookup aggregate
	aggrSpaceResponse, aggrSpaceErr := client.AggrSpaceGetIterRequest(aggregate)
	if aggrSpaceErr != nil {
		return aggrSpaceErr
	}

	// iterate over results
	if aggrSpaceResponse.Result.AttributesListPtr != nil {
		for _, aggrSpace := range aggrSpaceResponse.Result.AttributesListPtr.SpaceInformationPtr {
			aggrName := aggrSpace.Aggregate()
			if aggregate != aggrName {
				log.Debugf("Skipping " + aggrName)
				continue
			}

			log.WithFields(log.Fields{
				"aggrName":                            aggrName,
				"size":                                aggrSpace.AggregateSize(),
				"volumeFootprints":                    aggrSpace.VolumeFootprints(),
				"volumeFootprintsPercent":             aggrSpace.VolumeFootprintsPercent(),
				"usedIncludingSnapshotReserve":        aggrSpace.UsedIncludingSnapshotReserve(),
				"usedIncludingSnapshotReservePercent": aggrSpace.UsedIncludingSnapshotReservePercent(),
			}).Info("Dumping aggregate space")

			if limitAggregateUsage != "" {
				percentLimit, parseErr := strconv.ParseFloat(limitAggregateUsage, 64)
				if parseErr != nil {
					return parseErr
				}

				usedIncludingSnapshotReserve := float64(aggrSpace.UsedIncludingSnapshotReserve())
				aggregateSize := float64(aggrSpace.AggregateSize())

				spaceReserveIsThick := false
				if spaceReserve == "volume" {
					spaceReserveIsThick = true
				}

				if spaceReserveIsThick {
					// we SHOULD include the requestedSize in our computation
					percentUsedWithRequest := ((usedIncludingSnapshotReserve + requestedSize) / aggregateSize) * 100.0
					log.WithFields(log.Fields{
						"percentUsedWithRequest": percentUsedWithRequest,
						"percentLimit":           percentLimit,
						"spaceReserve":           spaceReserve,
					}).Debugf("Checking usage percentage limits")

					if percentUsedWithRequest >= percentLimit {
						errorMessage := fmt.Sprintf("aggregate usage of %.2f %% would exceed the limit of %.2f %%",
							percentUsedWithRequest, percentLimit)
						return errors.New(errorMessage)
					}
				} else {
					// we should NOT include the requestedSize in our computation
					percentUsedWithoutRequest := ((usedIncludingSnapshotReserve) / aggregateSize) * 100.0
					log.WithFields(log.Fields{
						"percentUsedWithoutRequest": percentUsedWithoutRequest,
						"percentLimit":              percentLimit,
						"spaceReserve":              spaceReserve,
					}).Debugf("Checking usage percentage limits")

					if percentUsedWithoutRequest >= percentLimit {
						errorMessage := fmt.Sprintf("aggregate usage of %.2f %% exceeds the limit of %.2f %%",
							percentUsedWithoutRequest, percentLimit)
						return errors.New(errorMessage)
					}
				}
			}

			log.Debugf("Request within specicifed limits, going to create.")
			return nil
		}
	}

	return errors.New("could not find aggregate, cannot check aggregate provisioning limits for " + aggregate)
}

func GetVolumeSize(sizeBytes uint64, config drivers.OntapStorageDriverConfig) (uint64, error) {

	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(config.Size)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return 0, fmt.Errorf("requested volume size (%d bytes) is too small; "+
			"the minimum volume size is %d bytes", sizeBytes, MinimumVolumeSizeBytes)
	}
	return sizeBytes, nil
}

func GetSnapshotReserve(snapshotPolicy, snapshotReserve string) (int, error) {

	if snapshotReserve != "" {
		// snapshotReserve defaults to "", so if it is explicitly set
		// (either in config or create options), honor the value.
		snapshotReserveInt64, err := strconv.ParseInt(snapshotReserve, 10, 64)
		if err != nil {
			return api.NumericalValueNotSet, err
		}
		return int(snapshotReserveInt64), nil
	} else {
		// If snapshotReserve isn't set, then look at snapshotPolicy.  If the policy is "none",
		// return 0.  Otherwise return -1, indicating that ONTAP should use its own default value.
		if snapshotPolicy == "none" {
			return 0, nil
		} else {
			return api.NumericalValueNotSet, nil
		}
	}
}

// EMSHeartbeat logs an ASUP message on a timer
// view them via filer::> event log show -severity NOTICE
func EMSHeartbeat(driver StorageDriver) {

	// log an informational message on a timer
	hostname, err := os.Hostname()
	if err != nil {
		log.Warnf("Could not determine hostname. %v", err)
		hostname = "unknown"
	}

	message, _ := json.Marshal(driver.GetTelemetry())

	emsResponse, err := driver.GetAPI().EmsAutosupportLog(
		strconv.Itoa(drivers.ConfigVersion), false, "heartbeat", hostname,
		string(message), 1, tridentconfig.OrchestratorName, 5)

	if err = api.GetError(emsResponse, err); err != nil {
		log.WithFields(log.Fields{
			"driver": driver.Name(),
			"error":  err,
		}).Error("Error logging EMS message.")
	} else {
		log.WithField("driver", driver.Name()).Debug("Logged EMS message.")
	}
}

const MSecPerHour = 1000 * 60 * 60 // millis * seconds * minutes

// probeForVolume polls for the ONTAP volume to appear, with backoff retry logic
func probeForVolume(name string, client *api.Client) error {
	checkVolumeExists := func() error {
		volExists, err := client.VolumeExists(name)
		if err != nil {
			return err
		}
		if !volExists {
			return fmt.Errorf("volume %v does not yet exist", name)
		}
		return nil
	}
	volumeExistsNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Volume not yet present, waiting.")
	}
	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 1 * time.Second
	volumeBackoff.Multiplier = 2
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 30 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeExists, volumeBackoff, volumeExistsNotify); err != nil {
		log.WithField("volume", name).Warnf("Could not find volume after %3.2f seconds.", volumeBackoff.MaxElapsedTime)
		return fmt.Errorf("volume %v does not exist", name)
	} else {
		log.WithField("volume", name).Debug("Volume found.")
		return nil
	}
}

// Create a volume clone
func CreateOntapClone(
	name, source, snapshot string, split bool, config *drivers.OntapStorageDriverConfig, client *api.Client,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateOntapClone",
			"Type":     "ontap_common",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"split":    split,
		}
		log.WithFields(fields).Debug(">>>> CreateOntapClone")
		defer log.WithFields(fields).Debug("<<<< CreateOntapClone")
	}

	// If the specified volume already exists, return an error
	volExists, err := client.VolumeExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return fmt.Errorf("volume %s already exists", name)
	}

	// If no specific snapshot was requested, create one
	if snapshot == "" {
		// This is golang being stupid: https://golang.org/pkg/time/#Time.Format
		snapshot = time.Now().UTC().Format("20060102T150405Z")
		snapResponse, err := client.SnapshotCreate(snapshot, source)
		if err = api.GetError(snapResponse, err); err != nil {
			return fmt.Errorf("error creating snapshot: %v", err)
		}
	}

	// Create the clone based on a snapshot
	cloneResponse, err := client.VolumeCloneCreate(name, source, snapshot)
	if err != nil {
		return fmt.Errorf("error creating clone: %v", err)
	}
	if zerr := api.NewZapiError(cloneResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			return fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, source)
		} else if zerr.IsFailedToLoadJobError() {
			fields := log.Fields{
				"zerr": zerr,
			}
			log.WithFields(fields).Warn("Problem encountered during the clone create operation, attempting to verify the clone was actually created")
			if volumeLookupError := probeForVolume(name, client); volumeLookupError != nil {
				return volumeLookupError
			}
		} else {
			return fmt.Errorf("error creating clone: %v", zerr)
		}
	}

	if config.StorageDriverName == drivers.OntapNASStorageDriverName {
		// Mount the new volume
		mountResponse, err := client.VolumeMount(name, "/"+name)
		if err = api.GetError(mountResponse, err); err != nil {
			return fmt.Errorf("error mounting volume to junction: %v", err)
		}
	}

	// Split the clone if requested
	if split {
		splitResponse, err := client.VolumeCloneSplitStart(name)
		if err = api.GetError(splitResponse, err); err != nil {
			return fmt.Errorf("error splitting clone: %v", err)
		}
	}

	return nil
}

// Return the list of snapshots associated with the named volume
func GetSnapshotList(name string, config *drivers.OntapStorageDriverConfig, client *api.Client) ([]storage.Snapshot, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetSnapshotList",
			"Type":   "ontap_common",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshotList")
		defer log.WithFields(fields).Debug("<<<< GetSnapshotList")
	}

	snapResponse, err := client.SnapshotGetByVolume(name)
	if err = api.GetError(snapResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	log.Debugf("Returned %v snapshots.", snapResponse.Result.NumRecords())
	snapshots := []storage.Snapshot{}

	if snapResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapResponse.Result.AttributesListPtr.SnapshotInfoPtr {

			log.WithFields(log.Fields{
				"name":       snap.Name(),
				"accessTime": snap.AccessTime(),
			}).Debug("Snapshot")

			// Time format: yyyy-mm-ddThh:mm:ssZ
			snapTime := time.Unix(int64(snap.AccessTime()), 0).UTC().Format("2006-01-02T15:04:05Z")

			snapshots = append(snapshots, storage.Snapshot{snap.Name(), snapTime})
		}
	}

	return snapshots, nil
}

// GetVolume checks for the existence of a volume.  It returns nil if the volume
// exists and an error if it does not (or the API call fails).
func GetVolume(name string, client *api.Client, config *drivers.OntapStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "GetVolume", "Type": "ontap_common"}
		log.WithFields(fields).Debug(">>>> GetVolume")
		defer log.WithFields(fields).Debug("<<<< GetVolume")
	}

	volExists, err := client.VolumeExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		log.WithField("flexvol", name).Debug("Flexvol not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// UpdateLoadSharingMirrors checks for the present of LS mirrors on the SVM root volume, and if
// present, starts an update and waits for them to become idle.
func UpdateLoadSharingMirrors(client *api.Client) {

	// We care about LS mirrors on the SVM root volume, so get the root volume name
	rootVolumeResponse, err := client.VolumeGetRootName()
	if err = api.GetError(rootVolumeResponse, err); err != nil {
		log.Warnf("Error getting SVM root volume. %v", err)
		return
	}
	rootVolume := rootVolumeResponse.Result.Volume()

	// Check for LS mirrors on the SVM root volume
	mirrorGetResponse, err := client.SnapmirrorGetLoadSharingMirrors(rootVolume)
	if err = api.GetError(rootVolumeResponse, err); err != nil {
		log.Warnf("Error getting load-sharing mirrors for SVM root volume. %v", err)
		return
	}
	if mirrorGetResponse.Result.NumRecords() == 0 {
		// None found, so nothing more to do
		log.WithField("rootVolume", rootVolume).Debug("SVM root volume has no load-sharing mirrors.")
		return
	}

	// One or more LS mirrors found, so issue an update
	if mirrorGetResponse.Result.AttributesListPtr != nil {
		mirrorSourceLocation := mirrorGetResponse.Result.AttributesListPtr.SnapmirrorInfoPtr[0].SourceLocation()
		_, err = client.SnapmirrorUpdateLoadSharingMirrors(mirrorSourceLocation)
		if err = api.GetError(rootVolumeResponse, err); err != nil {
			log.Warnf("Error updating load-sharing mirrors for SVM root volume. %v", err)
			return
		}
	} else {
		log.Warnf("Error updating load-sharing mirrors for SVM root volume. %v", err)
		return
	}

	// Wait for LS mirrors to become idle
	timeout := time.Now().Add(LSMirrorIdleTimeoutSecs * time.Second)
	for {
		time.Sleep(1 * time.Second)
		log.Debug("Load-sharing mirrors not yet idle, polling...")

		mirrorGetResponse, err = client.SnapmirrorGetLoadSharingMirrors(rootVolume)
		if err = api.GetError(rootVolumeResponse, err); err != nil {
			log.Warnf("Error getting load-sharing mirrors for SVM root volume. %v", err)
			break
		}
		if mirrorGetResponse.Result.NumRecords() == 0 {
			log.WithField("rootVolume", rootVolume).Debug("SVM root volume has no load-sharing mirrors.")
			break
		}

		// Ensure all mirrors are idle
		idle := true
		if mirrorGetResponse.Result.AttributesListPtr != nil {
			for _, mirror := range mirrorGetResponse.Result.AttributesListPtr.SnapmirrorInfoPtr {
				if mirror.RelationshipStatusPtr == nil || mirror.RelationshipStatus() != "idle" {
					idle = false
				}
			}
		}

		if idle {
			log.Debug("Load-sharing mirrors idle.")
			break
		}

		// Don't wait forever
		if time.Now().After(timeout) {
			log.Warning("Load-sharing mirrors not yet idle, giving up.")
			break
		}
	}
}

type ontapPerformanceClass string

const (
	ontapHDD    ontapPerformanceClass = "hdd"
	ontapHybrid ontapPerformanceClass = "hybrid"
	ontapSSD    ontapPerformanceClass = "ssd"
)

var ontapPerformanceClasses = map[ontapPerformanceClass]map[string]sa.Offer{
	ontapHDD:    {sa.Media: sa.NewStringOffer(sa.HDD)},
	ontapHybrid: {sa.Media: sa.NewStringOffer(sa.Hybrid)},
	ontapSSD:    {sa.Media: sa.NewStringOffer(sa.SSD)},
}

// getStorageBackendSpecsCommon discovers the aggregates assigned to the configured SVM, and it updates the specified Backend
// object with StoragePools and their associated attributes.
func getStorageBackendSpecsCommon(
	d StorageDriver, backend *storage.Backend, poolAttributes map[string]sa.Offer,
) (err error) {

	client := d.GetAPI()
	config := d.GetConfig()
	driverName := d.Name()

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := client.VserverGetAggregateNames()
	if err != nil {
		return
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", config.SVM)
		return
	}

	log.WithFields(log.Fields{
		"svm":   config.SVM,
		"pools": vserverAggrs,
	}).Debug("Read storage pools assigned to SVM.")

	// Define a storage pool for each of the SVM's aggregates
	storagePools := make(map[string]*storage.Pool)
	for _, aggrName := range vserverAggrs {
		storagePools[aggrName] = storage.NewStoragePool(backend, aggrName)
	}

	// Use all assigned aggregates unless 'aggregate' is set in the config
	if config.Aggregate != "" {

		// Make sure the configured aggregate is available to the SVM
		if _, ok := storagePools[config.Aggregate]; !ok {
			err = fmt.Errorf("the assigned aggregates for SVM %s do not include the configured aggregate %s",
				config.SVM, config.Aggregate)
			return
		}

		log.WithFields(log.Fields{
			"driverName": driverName,
			"aggregate":  config.Aggregate,
		}).Debug("Provisioning will be restricted to the aggregate set in the backend config.")

		storagePools = make(map[string]*storage.Pool)
		storagePools[config.Aggregate] = storage.NewStoragePool(backend, config.Aggregate)
	}

	// Update pools with aggregate info (i.e. MediaType) using the best means possible
	var aggrErr error
	if client.SupportsFeature(api.VServerShowAggr) {
		aggrErr = getVserverAggregateAttributes(d, &storagePools)
	} else {
		aggrErr = getClusterAggregateAttributes(d, &storagePools)
	}

	if zerr, ok := aggrErr.(api.ZapiError); ok && zerr.IsScopeError() {
		log.WithFields(log.Fields{
			"username": config.Username,
		}).Warn("User has insufficient privileges to obtain aggregate info. " +
			"Storage classes with physical attributes such as 'media' will not match pools on this backend.")
	} else if aggrErr != nil {
		log.Errorf("Could not obtain aggregate info; storage classes with physical attributes such as 'media' will"+
			" not match pools on this backend: %v.", aggrErr)
	}

	// Add attributes common to each pool and register pools with backend
	for _, pool := range storagePools {

		for attrName, offer := range poolAttributes {
			pool.Attributes[attrName] = offer
		}

		backend.AddStoragePool(pool)
	}

	return
}

// getVserverAggregateAttributes gets pool attributes using vserver-show-aggr-get-iter, which will only succeed on Data ONTAP 9 and later.
// If the aggregate attributes are read successfully, the pools passed to this function are updated accordingly.
func getVserverAggregateAttributes(d StorageDriver, storagePools *map[string]*storage.Pool) error {

	result, err := d.GetAPI().VserverShowAggrGetIterRequest()
	if err != nil {
		return err
	}
	if zerr := api.NewZapiError(result.Result); !zerr.IsPassed() {
		return zerr
	}

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.ShowAggregatesPtr {
			aggrName := string(aggr.AggregateName())
			aggrType := aggr.AggregateType()

			// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
			pool, ok := (*storagePools)[aggrName]
			if !ok {
				continue
			}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				log.WithFields(log.Fields{
					"aggregate": aggrName,
					"mediaType": aggrType,
				}).Debug("Aggregate has unknown performance characteristics.")

				continue
			}

			log.WithFields(log.Fields{
				"aggregate": aggrName,
				"mediaType": aggrType,
			}).Debug("Read aggregate attributes.")

			// Update the pool with the aggregate storage attributes
			for attrName, attr := range storageAttrs {
				pool.Attributes[attrName] = attr
			}
		}
	}

	return nil
}

// getClusterAggregateAttributes gets pool attributes using aggr-get-iter, which will only succeed for cluster-scoped users
// with adequate permissions.  If the aggregate attributes are read successfully, the pools passed to this function are updated
// accordingly.
func getClusterAggregateAttributes(d StorageDriver, storagePools *map[string]*storage.Pool) error {

	result, err := d.GetAPI().AggrGetIterRequest()
	if err != nil {
		return err
	}
	if zerr := api.NewZapiError(result.Result); !zerr.IsPassed() {
		return zerr
	}

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.AggrAttributesPtr {
			aggrName := aggr.AggregateName()
			aggrRaidAttrs := aggr.AggrRaidAttributes()
			aggrType := aggrRaidAttrs.AggregateType()

			// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
			pool, ok := (*storagePools)[aggrName]
			if !ok {
				continue
			}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				log.WithFields(log.Fields{
					"aggregate": aggrName,
					"mediaType": aggrType,
				}).Debug("Aggregate has unknown performance characteristics.")

				continue
			}

			log.WithFields(log.Fields{
				"aggregate": aggrName,
				"mediaType": aggrType,
			}).Debug("Read aggregate attributes.")

			// Update the pool with the aggregate storage attributes
			for attrName, attr := range storageAttrs {
				pool.Attributes[attrName] = attr
			}
		}
	}

	return nil
}

func getVolumeOptsCommon(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) map[string]string {
	opts := make(map[string]string)
	if pool != nil {
		opts["aggregate"] = pool.Name
	}
	if provisioningTypeReq, ok := requests[sa.ProvisioningType]; ok {
		if p, ok := provisioningTypeReq.Value().(string); ok {
			if p == "thin" {
				opts["spaceReserve"] = "none"
			} else if p == "thick" {
				// p will equal "thick" here
				opts["spaceReserve"] = "volume"
			} else {
				log.WithFields(log.Fields{
					"provisioner":      "ONTAP",
					"method":           "getVolumeOptsCommon",
					"provisioningType": provisioningTypeReq.Value(),
				}).Warnf("Expected 'thick' or 'thin' for %s; ignoring.",
					sa.ProvisioningType)
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner":      "ONTAP",
				"method":           "getVolumeOptsCommon",
				"provisioningType": provisioningTypeReq.Value(),
			}).Warnf("Expected string for %s; ignoring.", sa.ProvisioningType)
		}
	}
	if encryptionReq, ok := requests[sa.Encryption]; ok {
		if encryption, ok := encryptionReq.Value().(bool); ok {
			if encryption {
				opts["encryption"] = "true"
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner": "ONTAP",
				"method":      "getVolumeOptsCommon",
				"encryption":  encryptionReq.Value(),
			}).Warnf("Expected bool for %s; ignoring.", sa.Encryption)
		}
	}
	if volConfig.SnapshotPolicy != "" {
		opts["snapshotPolicy"] = volConfig.SnapshotPolicy
	}
	if volConfig.SnapshotReserve != "" {
		opts["snapshotReserve"] = volConfig.SnapshotReserve
	}
	if volConfig.UnixPermissions != "" {
		opts["unixPermissions"] = volConfig.UnixPermissions
	}
	if volConfig.SnapshotDir != "" {
		opts["snapshotDir"] = volConfig.SnapshotDir
	}
	if volConfig.ExportPolicy != "" {
		opts["exportPolicy"] = volConfig.ExportPolicy
	}
	if volConfig.SpaceReserve != "" {
		opts["spaceReserve"] = volConfig.SpaceReserve
	}
	if volConfig.SecurityStyle != "" {
		opts["securityStyle"] = volConfig.SecurityStyle
	}
	if volConfig.SplitOnClone != "" {
		opts["splitOnClone"] = volConfig.SplitOnClone
	}
	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}
	if volConfig.Encryption != "" {
		opts["encryption"] = volConfig.Encryption
	}

	return opts
}

func getInternalVolumeNameCommon(commonConfig *drivers.CommonStorageDriverConfig, name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *commonConfig.StoragePrefix + name
	} else {
		// With an external store, any transformation of the name is fine
		internal := drivers.GetCommonInternalVolumeName(commonConfig, name)
		internal = strings.Replace(internal, "-", "_", -1)  // ONTAP disallows hyphens
		internal = strings.Replace(internal, ".", "_", -1)  // ONTAP disallows periods
		internal = strings.Replace(internal, "__", "_", -1) // Remove any double underscores
		return internal
	}
}

func createPrepareCommon(d storage.Driver, volConfig *storage.VolumeConfig) bool {

	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}

	return true
}

func getExternalConfig(config drivers.OntapStorageDriverConfig) interface{} {

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.OntapStorageDriverConfig
	drivers.Clone(config, &cloneConfig)

	drivers.SanitizeCommonStorageDriverConfig(cloneConfig.CommonStorageDriverConfig)
	cloneConfig.Username = "" // redact the username
	cloneConfig.Password = "" // redact the password
	return cloneConfig
}

// resizeValidation performs needed validation checks prior to the resize operation.
func resizeValidation(name string, sizeBytes uint64,
	volumeExists func(string) (bool, error),
	volumeSize func(string) (int, error)) (uint64, error) {

	// Check that volume exists
	volExists, err := volumeExists(name)
	if err != nil {
		log.WithField("error", err).Errorf("Error checking for existing volume.")
		return 0, fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return 0, fmt.Errorf("volume %s does not exist", name)
	}

	// Check that current size is smaller than requested size
	volSize, err := volumeSize(name)
	if err != nil {
		log.WithField("error", err).Errorf("Error checking volume size.")
		return 0, fmt.Errorf("error occurred when checking volume size")
	}
	volSizeBytes := uint64(volSize)

	if sizeBytes < volSizeBytes {
		return 0, fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volSize)
	}

	return volSizeBytes, nil
}
