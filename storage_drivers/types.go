package storagedrivers

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/fake"
	sfapi "github.com/netapp/trident/storage_drivers/solidfire/api"
	"github.com/netapp/trident/utils"
)

// CommonStorageDriverConfig holds settings in common across all StorageDrivers
type CommonStorageDriverConfig struct {
	Version           int                   `json:"version"`
	StorageDriverName string                `json:"storageDriverName"`
	BackendName       string                `json:"backendName"`
	Debug             bool                  `json:"debug"`           // Unsupported!
	DebugTraceFlags   map[string]bool       `json:"debugTraceFlags"` // Example: {"api":false, "method":true}
	DisableDelete     bool                  `json:"disableDelete"`
	StoragePrefixRaw  json.RawMessage       `json:"storagePrefix,string"`
	StoragePrefix     *string               `json:"-"`
	SerialNumbers     []string              `json:"serialNumbers,omitEmpty"`
	DriverContext     trident.DriverContext `json:"-"`
	LimitVolumeSize   string                `json:"limitVolumeSize"`
}

type CommonStorageDriverConfigDefaults struct {
	Size string `json:"size"`
}

// ESeriesStorageDriverConfig holds settings for ESeriesStorageDriver
type ESeriesStorageDriverConfig struct {
	*CommonStorageDriverConfig

	// Web Proxy Services Info
	WebProxyHostname  string `json:"webProxyHostname"`
	WebProxyPort      string `json:"webProxyPort"`      // optional
	WebProxyUseHTTP   bool   `json:"webProxyUseHTTP"`   // optional
	WebProxyVerifyTLS bool   `json:"webProxyVerifyTLS"` // optional
	Username          string `json:"username"`
	Password          string `json:"password"`

	// Array Info
	ControllerA   string `json:"controllerA"`
	ControllerB   string `json:"controllerB"`
	PasswordArray string `json:"passwordArray"` //optional

	// Options
	PoolNameSearchPattern string `json:"poolNameSearchPattern"` //optional

	// Host Networking
	HostDataIPDeprecated string `json:"hostData_IP,omitempty"` // for backward compatibility only
	HostDataIP           string `json:"hostDataIP"`            // for iSCSI can be either port if multipathing is setup
	AccessGroup          string `json:"accessGroupName"`       // name for host group
	HostType             string `json:"hostType"`              // host type, default is 'linux_dm_mp'

	EseriesStorageDriverConfigDefaults `json:"defaults"`
}

type EseriesStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// OntapStorageDriverConfig holds settings for OntapStorageDrivers
type OntapStorageDriverConfig struct {
	*CommonStorageDriverConfig              // embedded types replicate all fields
	ManagementLIF                    string `json:"managementLIF"`
	DataLIF                          string `json:"dataLIF"`
	IgroupName                       string `json:"igroupName"`
	SVM                              string `json:"svm"`
	Username                         string `json:"username"`
	Password                         string `json:"password"`
	Aggregate                        string `json:"aggregate"`
	UsageHeartbeat                   string `json:"usageHeartbeat"`           // in hours, default to 24.0
	QtreePruneFlexvolsPeriod         string `json:"qtreePruneFlexvolsPeriod"` // in seconds, default to 600
	QtreeQuotaResizePeriod           string `json:"qtreeQuotaResizePeriod"`   // in seconds, default to 60
	NfsMountOptions                  string `json:"nfsMountOptions"`
	LimitAggregateUsage              string `json:"limitAggregateUsage"`
        QtreePoolVolUnixPermissions      string `json:"qtreePoolVolUnixPermissions"`            // format: "711"
        QtreePoolVolKeepIfEmpty          string `json:"qtreePoolVolKeepIfEmpty"`                // format: "1" or "0"
        QtreePoolVolPrefix               string `json:"qtreePoolVolPrefix"`                     // format: "TRIDENT_"
        QtreePoolVolSnapPolicyLookupPattern string `json:"qtreePoolVolSnapPolicyLookupPattern"` // format: "*"
        QtreePoolVolExportPolicy         string `json:"qtreePoolVolExportPolicy"`               // format: "EP_CUSTOM"
        QtreePoolVolMaxMumberOfQtrees    string `json:"qtreePoolVolMaxMumberOfQtrees"`          // format: "100"
        LimitQtreeSize                   string `json:"limitQtreeSize"`                         // format: "20Ti"
	OntapStorageDriverConfigDefaults `json:"defaults"`
}

type OntapStorageDriverConfigDefaults struct {
	SpaceReserve    string `json:"spaceReserve"`
	SnapshotPolicy  string `json:"snapshotPolicy"`
	SnapshotReserve string `json:"snapshotReserve"`
	SnapshotDir     string `json:"snapshotDir"`
	UnixPermissions string `json:"unixPermissions"`
	ExportPolicy    string `json:"exportPolicy"`
	SecurityStyle   string `json:"securityStyle"`
	SplitOnClone    string `json:"splitOnClone"`
	FileSystemType  string `json:"fileSystemType"`
	Encryption      string `json:"encryption"`
	CommonStorageDriverConfigDefaults
}

// SolidfireStorageDriverConfig holds settings for SolidfireStorageDrivers
type SolidfireStorageDriverConfig struct {
	*CommonStorageDriverConfig           // embedded types replicate all fields
	TenantName                           string
	EndPoint                             string
	SVIP                                 string
	InitiatorIFace                       string //iface to use of iSCSI initiator
	Types                                *[]sfapi.VolType
	LegacyNamePrefix                     string //name prefix used in earlier ndvp versions
	AccessGroups                         []int64
	UseCHAP                              bool
	DefaultBlockSize                     int64 //blocksize to use on create when not specified  (512|4096, 512 is default)
	SolidfireStorageDriverConfigDefaults `json:"defaults"`
}

type SolidfireStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

type AWSNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	APIURL          string `json:"apiURL"`
	APIKey          string `json:"apiKey"`
	APIRegion       string `json:"apiRegion"`
	SecretKey       string `json:"secretKey"`
	NfsMountOptions string `json:"nfsMountOptions"`
	AWSNFSStorageDriverPool
	Storage []AWSNFSStorageDriverPool `json:"storage"`
}

type AWSNFSStorageDriverPool struct {
	Labels                            map[string]string `json:"labels"`
	Region                            string            `json:"region"`
	Zone                              string            `json:"zone"`
	ServiceLevel                      string            `json:"serviceLevel"`
	AWSNFSStorageDriverConfigDefaults `json:"defaults"`
}

type AWSNFSStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotReserve string `json:"snapshotReserve"`
	CommonStorageDriverConfigDefaults
}

type FakeStorageDriverConfig struct {
	*CommonStorageDriverConfig
	Protocol trident.Protocol `json:"protocol"`
	// Pools are the modeled physical pools.  At least one is required.
	Pools map[string]*fake.StoragePool `json:"pools"`
	// Volumes are the modeled backend volumes that exist when the driver starts.  Optional.
	Volumes      []fake.Volume `json:"volumes"`
	InstanceName string        `json:"instanceName"`
	FakeStorageDriverPool
	Storage []FakeStorageDriverPool `json:"storage"`
}

type FakeStorageDriverPool struct {
	Labels                          map[string]string `json:"labels"`
	Region                          string            `json:"region"`
	Zone                            string            `json:"zone"`
	FakeStorageDriverConfigDefaults `json:"defaults"`
}

type FakeStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// ValidateCommonSettings attempts to "partially" decode the JSON into just the settings in CommonStorageDriverConfig
func ValidateCommonSettings(configJSON string) (*CommonStorageDriverConfig, error) {

	config := &CommonStorageDriverConfig{}

	// Decode configJSON into config object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not parse JSON configuration: %v", err)
	}

	// Load storage drivers and validate the one specified actually exists
	if config.StorageDriverName == "" {
		return nil, errors.New("missing storage driver name in configuration file")
	}

	// Validate config file version information
	if config.Version != ConfigVersion {
		return nil, fmt.Errorf("unexpected config file version; found %d, expected %d", config.Version, ConfigVersion)
	}

	// Warn about ignored fields in common config if any are set
	if config.DisableDelete {
		log.WithFields(log.Fields{
			"driverName": config.StorageDriverName,
		}).Warn("disableDelete set in backend config.  This will be ignored.")
	}
	if config.Debug {
		log.Warnf("The debug setting in the configuration file is now ignored; " +
			"use the command line --debug switch instead.")
	}

	// The storage prefix may have three states: nil (no prefix specified, drivers will use
	// a default prefix), "" (specified as an empty string, drivers will use no prefix), and
	// "<value>" (a prefix specified in the backend config file).  For historical reasons,
	// the value is serialized as a raw JSON string (a byte array), and it may take multiple
	// forms.  An empty byte array, or an array with the ASCII values {} or null, is interpreted
	// as nil (no prefix specified).  A byte array containing two double-quote characters ("")
	// is an empty string.  A byte array containing characters enclosed in double quotes is
	// a specified prefix.  Anything else is rejected as invalid.  The storage prefix is exposed
	// to the rest of the code in StoragePrefix; only serialization code such as this should
	// be concerned with StoragePrefixRaw.

	if len(config.StoragePrefixRaw) > 0 {
		rawPrefix := string(config.StoragePrefixRaw)
		if rawPrefix == "{}" || rawPrefix == "null" {
			config.StoragePrefix = nil
			log.Debugf("Storage prefix is %s, will use default prefix.", rawPrefix)
		} else if rawPrefix == "\"\"" {
			empty := ""
			config.StoragePrefix = &empty
			log.Debug("Storage prefix is empty, will use no prefix.")
		} else if strings.HasPrefix(rawPrefix, "\"") && strings.HasSuffix(rawPrefix, "\"") {
			prefix := string(config.StoragePrefixRaw[1 : len(config.StoragePrefixRaw)-1])
			config.StoragePrefix = &prefix
			log.WithField("storagePrefix", prefix).Debug("Parsed storage prefix.")
		} else {
			return nil, fmt.Errorf("invalid value for storage prefix: %v", config.StoragePrefixRaw)
		}
	} else {
		config.StoragePrefix = nil
		log.Debug("Storage prefix is absent, will use default prefix.")
	}

	// Validate volume size limit (if set)
	if config.LimitVolumeSize != "" {
		if _, err = utils.ConvertSizeToBytes(config.LimitVolumeSize); err != nil {
			return nil, fmt.Errorf("invalid value for limitVolumeSize: %v", config.LimitVolumeSize)
		}
	}

	log.Debugf("Parsed commonConfig: %+v", *config)

	return config, nil
}

func GetDefaultStoragePrefix(context trident.DriverContext) string {
	switch context {
	default:
		return ""
	case trident.ContextKubernetes, trident.ContextCSI:
		return DefaultTridentStoragePrefix
	case trident.ContextDocker:
		return DefaultDockerStoragePrefix
	}
}

func GetDefaultIgroupName(context trident.DriverContext) string {
	switch context {
	default:
		fallthrough
	case trident.ContextKubernetes, trident.ContextCSI:
		return DefaultTridentIgroupName
	case trident.ContextDocker:
		return DefaultDockerIgroupName
	}
}

func SanitizeCommonStorageDriverConfig(c *CommonStorageDriverConfig) {
	if c != nil && c.StoragePrefixRaw == nil {
		c.StoragePrefixRaw = json.RawMessage("{}")
	}
}

func GetCommonInternalVolumeName(c *CommonStorageDriverConfig, name string) string {

	prefixToUse := trident.OrchestratorName

	// If a prefix was specified in the configuration, use that.
	if c.StoragePrefix != nil {
		prefixToUse = *c.StoragePrefix
	}

	// Special case an empty prefix so that we don't get a delimiter in front.
	if prefixToUse == "" {
		return name
	}

	return fmt.Sprintf("%s-%s", prefixToUse, name)
}

// CheckVolumeSizeLimits if a limit has been set, ensures the requestedSize is under it.
func CheckVolumeSizeLimits(requestedSizeInt uint64, config *CommonStorageDriverConfig) (bool, uint64, error) {

	requestedSize := float64(requestedSizeInt)
	// if the user specified a limit for volume size, parse and enforce it
	limitVolumeSize := config.LimitVolumeSize
	log.WithFields(log.Fields{
		"limitVolumeSize": limitVolumeSize,
	}).Debugf("Limits")
	if limitVolumeSize == "" {
		log.Debugf("No limits specified, not limiting volume size")
		return false, 0, nil
	}

	volumeSizeLimit := uint64(0)
	volumeSizeLimitStr, parseErr := utils.ConvertSizeToBytes(limitVolumeSize)
	if parseErr != nil {
		return false, 0, fmt.Errorf("error parsing limitVolumeSize: %v", parseErr)
	}
	volumeSizeLimit, _ = strconv.ParseUint(volumeSizeLimitStr, 10, 64)

	log.WithFields(log.Fields{
		"limitVolumeSize":    limitVolumeSize,
		"volumeSizeLimit":    volumeSizeLimit,
		"requestedSizeBytes": requestedSize,
	}).Debugf("Comparing limits")

	if requestedSize > float64(volumeSizeLimit) {
		return true, volumeSizeLimit, fmt.Errorf("requested size: %1.f > the size limit: %d", requestedSize, volumeSizeLimit)
	}

	return true, volumeSizeLimit, nil
}

// Clone will create a copy of the source object and store it into the destination object (which must be a pointer)
func Clone(source, destination interface{}) {
	if reflect.TypeOf(destination).Kind() != reflect.Ptr {
		log.Error("storage_drivers.Clone, destination parameter must be a pointer")
	}

	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(source)
	dec.Decode(destination)
}

type BackendIneligibleError struct {
	message string
}

func (e *BackendIneligibleError) Error() string { return e.message }

func NewBackendIneligibleError(volumeName string, errors []error) error {
	messages := make([]string, 0)
	for _, err := range errors {
		messages = append(messages, err.Error())
	}

	return &BackendIneligibleError{
		message: fmt.Sprintf("backend cannot satisfy create request for volume %s: (%s)",
			volumeName, strings.Join(messages, "; ")),
	}
}

func IsBackendIneligibleError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*BackendIneligibleError)
	return ok
}

type VolumeExistsError struct {
	message string
}

func (e *VolumeExistsError) Error() string { return e.message }

func NewVolumeExistsError(name string) error {
	return &VolumeExistsError{
		message: fmt.Sprintf("volume %s already exists", name),
	}
}

func IsVolumeExistsError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*VolumeExistsError)
	return ok
}
