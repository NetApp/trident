// Copyright 2021 NetApp, Inc. All Rights Reserved.
package storagedrivers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/fake"
	sfapi "github.com/netapp/trident/storage_drivers/solidfire/api"
	"github.com/netapp/trident/utils"
)

// DriverConfig provides a common interface for storage config related operations
type DriverConfig interface {
	String() string
	GoString() string
	GetCredentials() (string, string, error)
	HasCredentials() bool
	SetBackendName(backendName string)
	InjectSecrets(secretMap map[string]string) error
	ExtractSecrets() map[string]string
	HideSensitiveWithSecretName(secretName string)
	GetAndHideSensitive(secretName string) map[string]string
	CheckForCRDControllerForbiddenAttributes() []string
	SpecOnlyValidation() error
}

func GetDriverConfigByName(driverName string) (DriverConfig, error) {

	var storageDriverConfig DriverConfig

	switch driverName {
	case OntapNASStorageDriverName:
		fallthrough
	case OntapNASQtreeStorageDriverName:
		fallthrough
	case OntapSANStorageDriverName:
		fallthrough
	case OntapSANEconomyStorageDriverName:
		fallthrough
	case OntapNASFlexGroupStorageDriverName:
		storageDriverConfig = &OntapStorageDriverConfig{}
	case SolidfireSANStorageDriverName:
		storageDriverConfig = &SolidfireStorageDriverConfig{}
	case EseriesIscsiStorageDriverName:
		storageDriverConfig = &ESeriesStorageDriverConfig{}
	case AWSNFSStorageDriverName:
		storageDriverConfig = &AWSNFSStorageDriverConfig{}
	case AzureNFSStorageDriverName:
		storageDriverConfig = &AzureNFSStorageDriverConfig{}
	case GCPNFSStorageDriverName:
		storageDriverConfig = &GCPNFSStorageDriverConfig{}
	case FakeStorageDriverName:
		storageDriverConfig = &FakeStorageDriverConfig{}
	default:
		return nil, fmt.Errorf("unknown storage driver: %v", driverName)
	}

	return storageDriverConfig, nil
}

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
	Credentials       map[string]string     `json:"credentials"`
}

type CommonStorageDriverConfigDefaults struct {
	Size string `json:"size"`
}

// Implement stringer interface for the CommonStorageDriverConfig driver
func (d CommonStorageDriverConfig) String() string {
	return ToString(&d, []string{"Credentials"}, nil)
}

// GetCredentials function returns secret name and type  (if set), otherwise empty strings
func (d *CommonStorageDriverConfig) GetCredentials() (string, string, error) {
	return getCredentialNameAndType(d.Credentials)
}

// HasCredentials returns if the credentials field is set, otherwise false
func (d *CommonStorageDriverConfig) HasCredentials() bool {
	return len(d.Credentials) != 0
}

// SetBackendName sets the backend name
func (d *CommonStorageDriverConfig) SetBackendName(backendName string) {
	d.BackendName = backendName
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

	EseriesStorageDriverPool
	Storage []EseriesStorageDriverPool `json:"storage"`
}

type EseriesStorageDriverPool struct {
	Labels                             map[string]string   `json:"labels"`
	Region                             string              `json:"region"`
	Zone                               string              `json:"zone"`
	SupportedTopologies                []map[string]string `json:"supportedTopologies"`
	EseriesStorageDriverConfigDefaults `json:"defaults"`
}

type EseriesStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the E-Series driver
func (d ESeriesStorageDriverConfig) String() string {
	return ToString(&d, []string{"Password", "PasswordArray", "Username"}, nil)
}

// Implement GoStringer interface for the ESeriesStorageDriverConfig driver
func (d ESeriesStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *ESeriesStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.Username, ok = secretMap[strings.ToLower("Username")]; !ok {
		return injectionError("Username")
	}
	if d.Password, ok = secretMap[strings.ToLower("Password")]; !ok {
		return injectionError("Password")
	}
	if d.PasswordArray, ok = secretMap[strings.ToLower("PasswordArray")]; !ok {
		return injectionError("PasswordArray")
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *ESeriesStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["Username"] = d.Username
	secretMap["Password"] = d.Password
	secretMap["PasswordArray"] = d.PasswordArray

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *ESeriesStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.Username = secretName
	d.Password = secretName
	d.PasswordArray = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *ESeriesStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d ESeriesStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d ESeriesStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

// OntapStorageDriverConfig holds settings for OntapStorageDrivers
type OntapStorageDriverConfig struct {
	*CommonStorageDriverConfig                // embedded types replicate all fields
	ManagementLIF                    string   `json:"managementLIF"`
	DataLIF                          string   `json:"dataLIF"`
	IgroupName                       string   `json:"igroupName"`
	SVM                              string   `json:"svm"`
	Username                         string   `json:"username"`
	Password                         string   `json:"password"`
	Aggregate                        string   `json:"aggregate"`
	UsageHeartbeat                   string   `json:"usageHeartbeat"`                   // in hours, default to 24.0
	QtreePruneFlexvolsPeriod         string   `json:"qtreePruneFlexvolsPeriod"`         // in seconds, default to 600
	QtreeQuotaResizePeriod           string   `json:"qtreeQuotaResizePeriod"`           // in seconds, default to 60
	QtreesPerFlexvol                 string   `json:"qtreesPerFlexvol"`                 // default to 200
	LUNsPerFlexvol                   string   `json:"lunsPerFlexvol"`                   // default to 100
	EmptyFlexvolDeferredDeletePeriod string   `json:"emptyFlexvolDeferredDeletePeriod"` // in seconds, default to 28800
	NfsMountOptions                  string   `json:"nfsMountOptions"`
	LimitAggregateUsage              string   `json:"limitAggregateUsage"`
	AutoExportPolicy                 bool     `json:"autoExportPolicy"`
	AutoExportCIDRs                  []string `json:"autoExportCIDRs"`
	OntapStorageDriverPool
	Storage                   []OntapStorageDriverPool `json:"storage"`
	UseCHAP                   bool                     `json:"useCHAP"`
	UseREST                   bool                     `json:"useREST"`
	ChapUsername              string                   `json:"chapUsername"`
	ChapInitiatorSecret       string                   `json:"chapInitiatorSecret"`
	ChapTargetUsername        string                   `json:"chapTargetUsername"`
	ChapTargetInitiatorSecret string                   `json:"chapTargetInitiatorSecret"`
	ClientPrivateKey          string                   `json:"clientPrivateKey"`
	ClientCertificate         string                   `json:"clientCertificate"`
	TrustedCACertificate      string                   `json:"trustedCACertificate"`
}

// String makes OntapStorageDriverConfig satisfy the Stringer interface.
func (d OntapStorageDriverConfig) String() string {
	return ToString(&d, GetOntapConfigRedactList(), nil)
}

// GoString makes OntapStorageDriverConfig satisfy the GoStringer interface.
func (d OntapStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *OntapStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.ClientPrivateKey, ok = secretMap[strings.ToLower("ClientPrivateKey")]; !ok || d.
		ClientPrivateKey == "" {
		if d.Username, ok = secretMap[strings.ToLower("Username")]; !ok {
			return injectionError("Username or ClientPrivateKey")
		}
		if d.Password, ok = secretMap[strings.ToLower("Password")]; !ok {
			return injectionError("Password")
		}
	}
	// CHAP settings
	if d.UseCHAP {
		if d.ChapUsername, ok = secretMap[strings.ToLower("ChapUsername")]; !ok {
			return injectionError("ChapUsername")
		}
		if d.ChapInitiatorSecret, ok = secretMap[strings.ToLower("ChapInitiatorSecret")]; !ok {
			return injectionError("ChapInitiatorSecret")
		}
		if d.ChapTargetUsername, ok = secretMap[strings.ToLower("ChapTargetUsername")]; !ok {
			return injectionError("ChapTargetUsername")
		}
		if d.ChapTargetInitiatorSecret, ok = secretMap[strings.ToLower("ChapTargetInitiatorSecret")]; !ok {
			return injectionError("ChapTargetInitiatorSecret")
		}
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *OntapStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["ClientPrivateKey"] = d.ClientPrivateKey
	secretMap["Username"] = d.Username
	secretMap["Password"] = d.Password

	if d.ClientPrivateKey != "" && d.Username != "" {
		log.Warn("Defaulting to certificate authentication, " +
			"it is not advised to have both certificate/key and username/password in backend file.")
	}

	// CHAP settings
	if d.UseCHAP {
		secretMap["ChapUsername"] = d.ChapUsername
		secretMap["ChapInitiatorSecret"] = d.ChapInitiatorSecret
		secretMap["ChapTargetUsername"] = d.ChapTargetUsername
		secretMap["ChapTargetInitiatorSecret"] = d.ChapTargetInitiatorSecret
	}

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *OntapStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {

	d.ClientPrivateKey = secretName
	d.Username = secretName
	d.Password = secretName

	// CHAP settings
	if d.UseCHAP {
		d.ChapUsername = secretName
		d.ChapInitiatorSecret = secretName
		d.ChapTargetUsername = secretName
		d.ChapTargetInitiatorSecret = secretName
	}
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *OntapStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d OntapStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d OntapStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type OntapStorageDriverPool struct {
	Labels                           map[string]string   `json:"labels"`
	Region                           string              `json:"region"`
	Zone                             string              `json:"zone"`
	SupportedTopologies              []map[string]string `json:"supportedTopologies"`
	OntapStorageDriverConfigDefaults `json:"defaults"`
}

type OntapStorageDriverConfigDefaults struct {
	SpaceAllocation   string `json:"spaceAllocation"`
	SpaceReserve      string `json:"spaceReserve"`
	SnapshotPolicy    string `json:"snapshotPolicy"`
	SnapshotReserve   string `json:"snapshotReserve"`
	SnapshotDir       string `json:"snapshotDir"`
	UnixPermissions   string `json:"unixPermissions"`
	ExportPolicy      string `json:"exportPolicy"`
	SecurityStyle     string `json:"securityStyle"`
	SplitOnClone      string `json:"splitOnClone"`
	FileSystemType    string `json:"fileSystemType"`
	Encryption        string `json:"encryption"`
	Mirroring         string `json:"mirroring"`
	TieringPolicy     string `json:"tieringPolicy"`
	QosPolicy         string `json:"qosPolicy"`
	AdaptiveQosPolicy string `json:"adaptiveQosPolicy"`
	CommonStorageDriverConfigDefaults
}

// SolidfireStorageDriverConfig holds settings for SolidfireStorageDrivers
type SolidfireStorageDriverConfig struct {
	*CommonStorageDriverConfig // embedded types replicate all fields
	TenantName                 string
	EndPoint                   string
	SVIP                       string
	InitiatorIFace             string //iface to use of iSCSI initiator
	Types                      *[]sfapi.VolType
	LegacyNamePrefix           string //name prefix used in earlier ndvp versions
	AccessGroups               []int64
	UseCHAP                    bool
	DefaultBlockSize           int64 //blocksize to use on create when not specified  (512|4096, 512 is default)

	SolidfireStorageDriverPool
	Storage []SolidfireStorageDriverPool `json:"storage"`
}

type SolidfireStorageDriverPool struct {
	Labels                               map[string]string   `json:"labels"`
	Region                               string              `json:"region"`
	Zone                                 string              `json:"zone"`
	Type                                 string              `json:"type"`
	SupportedTopologies                  []map[string]string `json:"supportedTopologies"`
	SolidfireStorageDriverConfigDefaults `json:"defaults"`
}

type SolidfireStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the Solidfire driver
func (d SolidfireStorageDriverConfig) String() string {
	return ToString(&d, []string{"TenantName", "EndPoint"}, nil)
}

// Implement GoStringer interface for the SolidfireStorageDriverConfig driver
func (d SolidfireStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *SolidfireStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.EndPoint, ok = secretMap[strings.ToLower("EndPoint")]; !ok {
		return injectionError("EndPoint")
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *SolidfireStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["EndPoint"] = d.EndPoint

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *SolidfireStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.EndPoint = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *SolidfireStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d SolidfireStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d SolidfireStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type AWSNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	APIURL              string `json:"apiURL"`
	APIKey              string `json:"apiKey"`
	APIRegion           string `json:"apiRegion"`
	SecretKey           string `json:"secretKey"`
	ProxyURL            string `json:"proxyURL"`
	NfsMountOptions     string `json:"nfsMountOptions"`
	VolumeCreateTimeout string `json:"volumeCreateTimeout"`
	AWSNFSStorageDriverPool
	Storage []AWSNFSStorageDriverPool `json:"storage"`
}

type AWSNFSStorageDriverPool struct {
	Labels                            map[string]string   `json:"labels"`
	Region                            string              `json:"region"`
	Zone                              string              `json:"zone"`
	ServiceLevel                      string              `json:"serviceLevel"`
	SupportedTopologies               []map[string]string `json:"supportedTopologies"`
	AWSNFSStorageDriverConfigDefaults `json:"defaults"`
}

type AWSNFSStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	SnapshotReserve string `json:"snapshotReserve"`
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the AWSNFSStorageDriverConfig driver
func (d AWSNFSStorageDriverConfig) String() string {
	return ToString(&d, []string{"APIURL", "APIKey", "SecretKey"}, nil)
}

// Implement GoStringer interface for the AWSNFSStorageDriverConfig driver
func (d AWSNFSStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *AWSNFSStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.APIKey, ok = secretMap[strings.ToLower("APIKey")]; !ok {
		return injectionError("APIKey")
	}
	if d.SecretKey, ok = secretMap[strings.ToLower("SecretKey")]; !ok {
		return injectionError("SecretKey")
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d AWSNFSStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["APIKey"] = d.APIKey
	secretMap["SecretKey"] = d.SecretKey

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *AWSNFSStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.APIKey = secretName
	d.SecretKey = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *AWSNFSStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d AWSNFSStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d AWSNFSStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type AzureNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	SubscriptionID  string `json:"subscriptionID"`
	TenantID        string `json:"tenantID"`
	ClientID        string `json:"clientID"`
	ClientSecret    string `json:"clientSecret"`
	NfsMountOptions string `json:"nfsMountOptions"`
	AzureNFSStorageDriverPool
	Storage             []AzureNFSStorageDriverPool `json:"storage"`
	VolumeCreateTimeout string                      `json:"volumeCreateTimeout"`
}

// Note that 'Region' and 'Zone' are internal specifiers, not related to Azure's
// 'Location' field.
type AzureNFSStorageDriverPool struct {
	Labels                              map[string]string   `json:"labels"`
	Region                              string              `json:"region"`
	Zone                                string              `json:"zone"`
	Location                            string              `json:"location"`
	ServiceLevel                        string              `json:"serviceLevel"`
	VirtualNetwork                      string              `json:"virtualNetwork"`
	Subnet                              string              `json:"subnet"`
	SupportedTopologies                 []map[string]string `json:"supportedTopologies"`
	AzureNFSStorageDriverConfigDefaults `json:"defaults"`
}

type AzureNFSStorageDriverConfigDefaults struct {
	ExportRule  string `json:"exportRule"`
	SnapshotDir string `json:"snapshotDir"`
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the AzureNFSStorageDriverConfig driver
func (d AzureNFSStorageDriverConfig) String() string {
	return ToString(&d, []string{"SubscriptionID", "TenantID", "ClientID", "ClientSecret"}, nil)
}

// Implement GoStringer interface for the AzureNFSStorageDriverConfig driver
func (d AzureNFSStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *AzureNFSStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.ClientID, ok = secretMap[strings.ToLower("ClientID")]; !ok {
		return injectionError("ClientID")
	}
	if d.ClientSecret, ok = secretMap[strings.ToLower("ClientSecret")]; !ok {
		return injectionError("ClientSecret")
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *AzureNFSStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["ClientID"] = d.ClientID
	secretMap["ClientSecret"] = d.ClientSecret

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *AzureNFSStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.ClientID = secretName
	d.ClientSecret = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *AzureNFSStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d AzureNFSStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d AzureNFSStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type GCPNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	ProjectNumber       string        `json:"projectNumber"`
	HostProjectNumber   string        `json:"hostProjectNumber"`
	APIKey              GCPPrivateKey `json:"apiKey"`
	APIRegion           string        `json:"apiRegion"`
	APIURL              string        `json:"apiURL"`
	APIAudienceURL      string        `json:"apiAudienceURL"`
	ProxyURL            string        `json:"proxyURL"`
	NfsMountOptions     string        `json:"nfsMountOptions"`
	VolumeCreateTimeout string        `json:"volumeCreateTimeout"`
	GCPNFSStorageDriverPool
	Storage []GCPNFSStorageDriverPool `json:"storage"`
}

type GCPNFSStorageDriverPool struct {
	Labels                            map[string]string   `json:"labels"`
	Region                            string              `json:"region"`
	Zone                              string              `json:"zone"`
	ServiceLevel                      string              `json:"serviceLevel"`
	StorageClass                      string              `json:"storageClass"`
	Network                           string              `json:"network"`
	SupportedTopologies               []map[string]string `json:"supportedTopologies"`
	GCPNFSStorageDriverConfigDefaults `json:"defaults"`
}

type GCPNFSStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	SnapshotReserve string `json:"snapshotReserve"`
	CommonStorageDriverConfigDefaults
}

type GCPPrivateKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// Implement stringer interface for the GCPNFSStorageDriverConfig driver
func (d GCPNFSStorageDriverConfig) String() string {
	return ToString(&d, []string{"ProjectNumber", "HostProjectNumber", "APIKey"}, nil)
}

// Implement GoStringer interface for the GCPNFSStorageDriverConfig driver
func (d GCPNFSStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *GCPNFSStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {

	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	if d.APIKey.PrivateKey, ok = secretMap[strings.ToLower("Private_Key")]; !ok {
		return injectionError("Private_Key")
	}
	if d.APIKey.PrivateKeyID, ok = secretMap[strings.ToLower("Private_Key_ID")]; !ok {
		return injectionError("Private_Key_ID")
	}

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *GCPNFSStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["Private_Key"] = d.APIKey.PrivateKey
	secretMap["Private_Key_ID"] = d.APIKey.PrivateKeyID

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *GCPNFSStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.APIKey.PrivateKey = secretName
	d.APIKey.PrivateKeyID = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *GCPNFSStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d GCPNFSStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d GCPNFSStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type FakeStorageDriverConfig struct {
	*CommonStorageDriverConfig
	Protocol trident.Protocol `json:"protocol"`
	// Pools are the modeled physical pools.  At least one is required.
	Pools map[string]*fake.StoragePool `json:"pools"`
	// Volumes are the modeled backend volumes that exist when the driver starts.  Optional.
	Volumes      []fake.Volume           `json:"volumes"`
	InstanceName string                  `json:"instanceName"`
	Storage      []FakeStorageDriverPool `json:"storage"`
	Username     string                  `json:"username"`
	Password     string                  `json:"password"`
	FakeStorageDriverPool
}

// Implement Stringer interface for the FakeStorageDriverConfig driver
func (d FakeStorageDriverConfig) String() string {
	return ToString(&d, []string{"Username", "Password"}, nil)
}

// Implement GoStringer interface for the FakeStorageDriverConfig driver
func (d FakeStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *FakeStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {
	// Nothing to do

	return nil
}

// ExtractSecrets function builds a map of any sensitive fields it contains (credentials, etc.),
// and returns the the map.
func (d *FakeStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["Username"] = d.Username
	secretMap["Password"] = d.Password

	return secretMap
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *FakeStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.Username = secretName
	d.Password = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *FakeStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	return map[string]string{}
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d FakeStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d FakeStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
	}

	if !d.HasCredentials() {
		return fmt.Errorf("input is missing the credentials field")
	}

	return nil
}

type FakeStorageDriverPool struct {
	Labels                          map[string]string   `json:"labels"`
	Region                          string              `json:"region"`
	Zone                            string              `json:"zone"`
	SupportedTopologies             []map[string]string `json:"supportedTopologies"`
	FakeStorageDriverConfigDefaults `json:"defaults"`
}

type FakeStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

type BackendIneligibleError struct {
	message                 string
	ineligiblePhysicalPools []string
}

func (e *BackendIneligibleError) Error() string { return e.message }
func (e *BackendIneligibleError) getIneligiblePhysicalPools() []string {
	return e.ineligiblePhysicalPools
}

func NewBackendIneligibleError(volumeName string, errors []error, ineligiblePhysicalPoolNames []string) error {
	messages := make([]string, 0)
	for _, err := range errors {
		messages = append(messages, err.Error())
	}

	return &BackendIneligibleError{
		message: fmt.Sprintf("backend cannot satisfy create request for volume %s: (%s)",
			volumeName, strings.Join(messages, "; ")),
		ineligiblePhysicalPools: ineligiblePhysicalPoolNames,
	}
}

func IsBackendIneligibleError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*BackendIneligibleError)
	return ok
}

func GetIneligiblePhysicalPoolNames(err error) (error, []string) {
	if IsBackendIneligibleError(err) {
		return nil, err.(*BackendIneligibleError).getIneligiblePhysicalPools()
	}
	return fmt.Errorf("this method is applicable to BackendIneligibleError type only"), nil
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

// ToString identifies attributes of a struct, stringifies them such that they can be consumed by the
// struct's stringer interface, and redacts elements specified in the redactList.
func ToString(structPointer interface{}, redactList []string, configVal interface{}) (out string) {

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in types#ToString; err: %v", r)
			out = "<panic>"
		}
	}()

	elements := reflect.ValueOf(structPointer).Elem()

	var output strings.Builder

	for i := 0; i < elements.NumField(); i++ {
		fieldName := elements.Type().Field(i).Name
		switch {
		case fieldName == "Config" && configVal != nil:
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, configVal))
		case utils.SliceContainsString(redactList, fieldName):
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, REDACTED))
		default:
			output.WriteString(fmt.Sprintf("%v:%#v ", fieldName, elements.Field(i)))
		}
	}

	out = output.String()
	return
}

func injectionError(fieldName string) error {
	return fmt.Errorf("%s field missing from backend secrets", fieldName)
}

// getCredentialNameAndType return secret name and type (if set) otherwise empty strings
func getCredentialNameAndType(credentials map[string]string) (string, string, error) {

	if len(credentials) == 0 {
		return "", "", nil
	}

	// Ensure Credentials does not contain any invalid key/value pair - this check ensures
	// we can expand this list in future without any risk
	credentialKeys := make([]string, 0, len(credentials))

	for k, _ := range credentials {
		credentialKeys = append(credentialKeys, k)
	}

	allowedCredentialKeys := []string{KeyName, KeyType}

	var invalidKeys []string
	for _, key := range credentialKeys {
		if !utils.SliceContainsString(allowedCredentialKeys, key) {
			invalidKeys = append(invalidKeys, key)
		}
	}

	if len(invalidKeys) > 0 {
		return "", "", fmt.Errorf("credentials field contains invalid fields '%v' attribute", invalidKeys)
	}

	secretName, ok := credentials[KeyName]
	if !ok {
		return "", "", fmt.Errorf("credentials field is missing 'name' attribute")
	}

	secretStore, ok := credentials[KeyType]
	if !ok {
		// if type is missing default to K8s secret
		secretStore = string(CredentialStoreK8sSecret)
	}

	if secretStore != string(CredentialStoreK8sSecret) {
		return "", "", fmt.Errorf("credentials field does not support type '%s'", secretStore)
	}

	return secretName, secretStore, nil
}

func checkMapContainsAttributes(forbiddenMap map[string]string) []string {
	var forbiddenList []string
	for key, value := range forbiddenMap {
		if value != "" {
			forbiddenList = append(forbiddenList, key)
		}
	}

	return forbiddenList
}
