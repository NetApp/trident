package storagedrivers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/fake"
	sfapi "github.com/netapp/trident/storage_drivers/solidfire/api"
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
	SerialNumbers     []string              `json:"-"`
	DriverContext     trident.DriverContext `json:"-"`
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
	OntapStorageDriverConfigDefaults `json:"defaults"`
}

type OntapStorageDriverConfigDefaults struct {
	SpaceReserve    string `json:"spaceReserve"`
	SnapshotPolicy  string `json:"snapshotPolicy"`
	UnixPermissions string `json:"unixPermissions"`
	SnapshotDir     string `json:"snapshotDir"`
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

type FakeStorageDriverConfig struct {
	*CommonStorageDriverConfig
	Protocol trident.Protocol `json:"protocol"`
	// pools represents the possible buckets into which a given volume should go
	Pools                           map[string]*fake.StoragePool `json:"pools"`
	InstanceName                    string                       `json:"instanceName"`
	FakeStorageDriverConfigDefaults `json:"defaults"`
}

type FakeStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// ValidateCommonSettings attempts to "partially" decode the JSON into just the settings in CommonStorageDriverConfig
func ValidateCommonSettings(configJSON string) (*CommonStorageDriverConfig, error) {
	log.Debugf("config: %s", configJSON)
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

	log.Debugf("Parsed commonConfig: %+v", *config)

	return config, nil
}

func GetDefaultStoragePrefix(context trident.DriverContext) string {
	switch context {
	default:
		fallthrough
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

type CommonStorageDriverConfigExternal struct {
	Version           int      `json:"version"`
	StorageDriverName string   `json:"storageDriverName"`
	StoragePrefix     *string  `json:"storagePrefix"`
	SerialNumbers     []string `json:"serialNumbers"`
}

func SanitizeCommonStorageDriverConfig(c *CommonStorageDriverConfig) {
	if c.StoragePrefixRaw == nil {
		c.StoragePrefixRaw = json.RawMessage("{}")
	}
}

func GetCommonStorageDriverConfigExternal(
	c *CommonStorageDriverConfig,
) *CommonStorageDriverConfigExternal {

	SanitizeCommonStorageDriverConfig(c)

	return &CommonStorageDriverConfigExternal{
		Version:           c.Version,
		StorageDriverName: c.StorageDriverName,
		StoragePrefix:     c.StoragePrefix,
		SerialNumbers:     c.SerialNumbers,
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
