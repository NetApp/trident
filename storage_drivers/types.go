// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"encoding/json"
	"fmt"
	"strings"

	trident "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
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
	ResetSecrets()
	HideSensitiveWithSecretName(secretName string)
	GetAndHideSensitive(secretName string) map[string]string
	CheckForCRDControllerForbiddenAttributes() []string
	SpecOnlyValidation() error
}

func GetDriverConfigByName(driverName string) (DriverConfig, error) {
	var storageDriverConfig DriverConfig

	switch driverName {
	case trident.OntapNASStorageDriverName:
		fallthrough
	case trident.OntapNASQtreeStorageDriverName:
		fallthrough
	case trident.OntapSANStorageDriverName:
		fallthrough
	case trident.OntapSANEconomyStorageDriverName:
		fallthrough
	case trident.OntapNASFlexGroupStorageDriverName:
		storageDriverConfig = &OntapStorageDriverConfig{}
	case trident.SolidfireSANStorageDriverName:
		storageDriverConfig = &SolidfireStorageDriverConfig{}
	case trident.AzureNASStorageDriverName:
		fallthrough
	case trident.AzureNASBlockStorageDriverName:
		storageDriverConfig = &AzureNASStorageDriverConfig{}
	case trident.GCPNFSStorageDriverName:
		storageDriverConfig = &GCPNFSStorageDriverConfig{}
	case trident.GCNVNASStorageDriverName:
		storageDriverConfig = &GCNVNASStorageDriverConfig{}
	case trident.FakeStorageDriverName:
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
	BackendPools      []string              `json:"backendPools,omitEmpty"`
	DriverContext     trident.DriverContext `json:"-"`
	LimitVolumeSize   string                `json:"limitVolumeSize"`
	Credentials       map[string]string     `json:"credentials"`
	UserState         string                `json:"userState"`
}

type CommonStorageDriverConfigDefaults struct {
	Size         string `json:"size"`
	NameTemplate string `json:"nameTemplate"`
}

// Implement stringer interface for the CommonStorageDriverConfig driver
func (d CommonStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, []string{"Credentials"}, nil)
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

// OntapStorageDriverConfig holds settings for OntapStorageDrivers
type OntapStorageDriverConfig struct {
	*CommonStorageDriverConfig                  // embedded types replicate all fields
	AWSConfig                        *AWSConfig `json:"aws,omitEmpty"` // AWS-specific config attributes
	ManagementLIF                    string     `json:"managementLIF"`
	DataLIF                          string     `json:"dataLIF"`
	IgroupName                       string     `json:"igroupName"`
	SVM                              string     `json:"svm"`
	Username                         string     `json:"username"`
	Password                         string     `json:"password"`
	Aggregate                        string     `json:"aggregate"`
	UsageHeartbeat                   string     `json:"usageHeartbeat"`                   // in hours, default to 24.0
	QtreePruneFlexvolsPeriod         string     `json:"qtreePruneFlexvolsPeriod"`         // in seconds, default to 600
	QtreeQuotaResizePeriod           string     `json:"qtreeQuotaResizePeriod"`           // in seconds, default to 60
	QtreesPerFlexvol                 string     `json:"qtreesPerFlexvol"`                 // default to 200
	LUNsPerFlexvol                   string     `json:"lunsPerFlexvol"`                   // default to 100
	EmptyFlexvolDeferredDeletePeriod string     `json:"emptyFlexvolDeferredDeletePeriod"` // in seconds, default to 28800
	CloneSplitDelay                  string     `json:"cloneSplitDelay,omitEmpty"`        // in seconds, default to 10
	NfsMountOptions                  string     `json:"nfsMountOptions"`
	LimitAggregateUsage              string     `json:"limitAggregateUsage"`
	LimitVolumePoolSize              string     `json:"limitVolumePoolSize"`
	AutoExportPolicy                 bool       `json:"autoExportPolicy"`
	AutoExportCIDRs                  []string   `json:"autoExportCIDRs"`
	OntapStorageDriverPool
	Storage                   []OntapStorageDriverPool `json:"storage"`
	UseCHAP                   bool                     `json:"useCHAP"`
	UseREST                   *bool                    `json:"useREST"`
	ChapUsername              string                   `json:"chapUsername"`
	ChapInitiatorSecret       string                   `json:"chapInitiatorSecret"`
	ChapTargetUsername        string                   `json:"chapTargetUsername"`
	ChapTargetInitiatorSecret string                   `json:"chapTargetInitiatorSecret"`
	ClientPrivateKey          string                   `json:"clientPrivateKey"`
	ClientCertificate         string                   `json:"clientCertificate"`
	TrustedCACertificate      string                   `json:"trustedCACertificate"`
	ReplicationPolicy         string                   `json:"replicationPolicy"`
	ReplicationSchedule       string                   `json:"replicationSchedule"`
	FlexGroupAggregateList    []string                 `json:"flexgroupAggregateList"`
}

type OntapStorageDriverPool struct {
	Labels                           map[string]string   `json:"labels"`
	Region                           string              `json:"region"`
	Zone                             string              `json:"zone"`
	SupportedTopologies              []map[string]string `json:"supportedTopologies"`
	NASType                          string              `json:"nasType"`
	SANType                          string              `json:"sanType"`
	SMBShare                         string              `json:"smbShare"`
	OntapStorageDriverConfigDefaults `json:"defaults"`
}

// AWSConfig holds settings for ONTAP drivers used with AWS, including FSx for NetApp ONTAP.
type AWSConfig struct {
	APIRegion       string `json:"apiRegion"`
	FSxFilesystemID string `json:"fsxFilesystemID"`
	APIKey          string `json:"apiKey"`
	SecretKey       string `json:"secretKey"`
}

// StorageBackendPool is a type constraint that enables drivers to generically report non-overlapping storage pools
// within a backend.
type StorageBackendPool interface {
	OntapFlexGroupStorageBackendPool | OntapStorageBackendPool | OntapEconomyStorageBackendPool |
		ANFStorageBackendPool | ANFSubvolumeStorageBackendPool | SolidfireStorageBackendPool |
		GCPNFSStorageBackendPool | GCNVNASStorageBackendPool
}

// OntapFlexGroupStorageBackendPool is a non-overlapping section of an ONTAP flexgroup backend that may be used for
// provisioning storage.
type OntapFlexGroupStorageBackendPool struct {
	SvmUUID string `json:"svmUUID"`
}

// OntapStorageBackendPool is a non-overlapping section of an ONTAP backend that may be used for provisioning storage.
type OntapStorageBackendPool struct {
	SvmUUID   string `json:"svmUUID"`
	Aggregate string `json:"aggregate"`
}

// OntapEconomyStorageBackendPool is a non-overlapping section of an ONTAP economy backend that may be used for
// provisioning storage.
type OntapEconomyStorageBackendPool struct {
	SvmUUID       string `json:"svmUUID"`
	Aggregate     string `json:"aggregate"`
	FlexVolPrefix string `json:"flexVolPrefix"`
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
	LUKSEncryption    string `json:"LUKSEncryption"`
	Mirroring         string `json:"mirroring"`
	TieringPolicy     string `json:"tieringPolicy"`
	QosPolicy         string `json:"qosPolicy"`
	AdaptiveQosPolicy string `json:"adaptiveQosPolicy"`
	CommonStorageDriverConfigDefaults
}

// String makes OntapStorageDriverConfig satisfy the Stringer interface.
func (d OntapStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, GetOntapConfigRedactList(), nil)
}

// GoString makes OntapStorageDriverConfig satisfy the GoStringer interface.
func (d OntapStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *OntapStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {
	// NOTE: When the backend secrets are read in the CRD persistance layer they are converted to lower-case.

	var ok bool
	// Inject the credentials from the secretMap into the driver's config
	if _, ok = secretMap[strings.ToLower("ClientPrivateKey")]; ok {
		d.ClientPrivateKey = secretMap[strings.ToLower("ClientPrivateKey")]
	}
	if _, ok = secretMap[strings.ToLower("Username")]; ok {
		d.Username = secretMap[strings.ToLower("Username")]
	}
	if _, ok = secretMap[strings.ToLower("Password")]; ok {
		d.Password = secretMap[strings.ToLower("Password")]
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
		Log().Warn("Defaulting to certificate authentication, " +
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

// ResetSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *OntapStorageDriverConfig) ResetSecrets() {
	d.ClientPrivateKey = ""
	d.Username = ""
	d.Password = ""

	// CHAP settings
	if d.UseCHAP {
		d.ChapUsername = ""
		d.ChapInitiatorSecret = ""
		d.ChapTargetUsername = ""
		d.ChapTargetInitiatorSecret = ""
	}
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *OntapStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	if d.ClientPrivateKey != "" {
		d.ClientPrivateKey = secretName
	}
	if d.Username != "" {
		d.Username = secretName
		d.Password = secretName
	}

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

// SolidfireStorageDriverConfig holds settings for SolidfireStorageDrivers
type SolidfireStorageDriverConfig struct {
	*CommonStorageDriverConfig // embedded types replicate all fields
	TenantName                 string
	EndPoint                   string
	SVIP                       string
	InitiatorIFace             string // iface to use of iSCSI initiator
	Types                      *[]sfapi.VolType
	LegacyNamePrefix           string // name prefix used in earlier ndvp versions
	AccessGroups               []int64
	UseCHAP                    bool
	DefaultBlockSize           int64 // blocksize to use on create when not specified  (512|4096, 512 is default)

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

// SolidfireStorageBackendPool is a non-overlapping section of a SolidFire backend that may be used for
// provisioning storage.
type SolidfireStorageBackendPool struct {
	AccountID  int64  `json:"accountID,string"`
	TenantName string `json:"tenantName"`
}

type SolidfireStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the Solidfire driver
func (d SolidfireStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, []string{"TenantName", "EndPoint"}, nil)
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

// RemoveSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *SolidfireStorageDriverConfig) ResetSecrets() {
	d.EndPoint = ""
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

type AzureNASStorageDriverConfig struct {
	*CommonStorageDriverConfig
	SubscriptionID         string            `json:"subscriptionID"`
	TenantID               string            `json:"tenantID"`
	ClientID               string            `json:"clientID"`
	ClientSecret           string            `json:"clientSecret"`
	Location               string            `json:"location"`
	NfsMountOptions        string            `json:"nfsMountOptions"`
	VolumeCreateTimeout    string            `json:"volumeCreateTimeout"`
	SDKTimeout             string            `json:"sdkTimeout"`
	MaxCacheAge            string            `json:"maxCacheAge"`
	CustomerEncryptionKeys map[string]string `json:"customerEncryptionKeys"`

	AzureNASStorageDriverPool
	Storage []AzureNASStorageDriverPool `json:"storage"`
}

// AzureNASStorageDriverPool is the virtual pool definition for the ANF driver.  Note that 'Region' and 'Zone'
// are internal specifiers, not related to Azure's 'Location' field.
type AzureNASStorageDriverPool struct {
	Labels                              map[string]string   `json:"labels"`
	Region                              string              `json:"region"`
	Zone                                string              `json:"zone"`
	ServiceLevel                        string              `json:"serviceLevel"`
	VirtualNetwork                      string              `json:"virtualNetwork"`
	Subnet                              string              `json:"subnet"`
	NetworkFeatures                     string              `json:"networkFeatures"`
	SupportedTopologies                 []map[string]string `json:"supportedTopologies"`
	ResourceGroups                      []string            `json:"resourceGroups"`
	NetappAccounts                      []string            `json:"netappAccounts"`
	CapacityPools                       []string            `json:"capacityPools"`
	FilePoolVolumes                     []string            `json:"filePoolVolumes"`
	NASType                             string              `json:"nasType"`
	Kerberos                            string              `json:"kerberos"`
	AzureNASStorageDriverConfigDefaults `json:"defaults"`
}

// ANFStorageBackendPool is a non-overlapping section of an Azure backend that may be used for provisioning storage.
type ANFStorageBackendPool struct {
	SubscriptionID string `json:"subscriptionID"`
	ResourceGroup  string `json:"resourceGroup"`
	NetappAccount  string `json:"netappAccount"`
	Location       string `json:"location"`
	CapacityPool   string `json:"capacityPool"`
}

// ANFSubvolumeStorageBackendPool is a non-overlapping section of an Azure file backend that may be used for
// provisioning storage.
type ANFSubvolumeStorageBackendPool struct {
	SubscriptionID string `json:"subscriptionID"`
	Location       string `json:"location"`
	FilePoolVolume string `json:"filePoolVolume"`
}

type AzureNASStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	UnixPermissions string `json:"unixPermissions"`
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the AzureNASStorageDriverConfig driver
func (d AzureNASStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, []string{"SubscriptionID", "TenantID", "ClientID", "ClientSecret"}, nil)
}

// Implement GoStringer interface for the AzureNASStorageDriverConfig driver
func (d AzureNASStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *AzureNASStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {
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
func (d *AzureNASStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["ClientID"] = d.ClientID
	secretMap["ClientSecret"] = d.ClientSecret

	return secretMap
}

// RemoveSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *AzureNASStorageDriverConfig) ResetSecrets() {
	d.ClientID = ""
	d.ClientSecret = ""
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *AzureNASStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.ClientID = secretName
	d.ClientSecret = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *AzureNASStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d AzureNASStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d AzureNASStorageDriverConfig) SpecOnlyValidation() error {
	if forbiddenList := d.CheckForCRDControllerForbiddenAttributes(); len(forbiddenList) > 0 {
		return fmt.Errorf("input contains forbidden attributes: %v", forbiddenList)
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
	StoragePools                      []string            `json:"storagePools"`
	Network                           string              `json:"network"`
	SupportedTopologies               []map[string]string `json:"supportedTopologies"`
	GCPNFSStorageDriverConfigDefaults `json:"defaults"`
}

// GCPNFSStorageBackendPool is a non-overlapping section of a GCP backend that may be used for provisioning storage.
type GCPNFSStorageBackendPool struct {
	ProjectNumber string `json:"projectNumber"`
	APIRegion     string `json:"apiRegion"`
	ServiceLevel  string `json:"serviceLevel"`
	StoragePool   string `json:"storagePool"`
}

type GCPNFSStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	SnapshotReserve string `json:"snapshotReserve"`
	UnixPermissions string `json:"unixPermissions"`
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
	return utils.ToStringRedacted(&d, []string{"ProjectNumber", "HostProjectNumber", "APIKey"}, nil)
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

// RemoveSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *GCPNFSStorageDriverConfig) ResetSecrets() {
	d.APIKey.PrivateKey = ""
	d.APIKey.PrivateKeyID = ""
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

type GCNVNASStorageDriverConfig struct {
	*CommonStorageDriverConfig
	ProjectNumber       string        `json:"projectNumber"`
	Location            string        `json:"location"`
	APIKey              GCPPrivateKey `json:"apiKey"`
	NFSMountOptions     string        `json:"nfsMountOptions"`
	VolumeCreateTimeout string        `json:"volumeCreateTimeout"`
	SDKTimeout          string        `json:"sdkTimeout"`
	MaxCacheAge         string        `json:"maxCacheAge"`
	NASType             string        `json:"nasType"`
	GCNVNASStorageDriverPool
	Storage []GCNVNASStorageDriverPool `json:"storage"`
}

type GCNVNASStorageDriverPool struct {
	Labels                             map[string]string   `json:"labels"`
	Region                             string              `json:"region"`
	Zone                               string              `json:"zone"`
	ServiceLevel                       string              `json:"serviceLevel"`
	StorageClass                       string              `json:"storageClass"`
	StoragePools                       []string            `json:"storagePools"`
	Network                            string              `json:"network"`
	SupportedTopologies                []map[string]string `json:"supportedTopologies"`
	GCNVNASStorageDriverConfigDefaults `json:"defaults"`
}

// GCNVNASStorageBackendPool is a non-overlapping section of a GCNV backend that may be used for provisioning storage.
type GCNVNASStorageBackendPool struct {
	ProjectNumber string `json:"projectNumber"`
	Location      string `json:"location"`
	StoragePool   string `json:"storagePool"`
}

type GCNVNASStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	SnapshotReserve string `json:"snapshotReserve"`
	UnixPermissions string `json:"unixPermissions"`
	CommonStorageDriverConfigDefaults
}

// Implement stringer interface for the GCNVNASStorageDriverConfig driver
func (d GCNVNASStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, []string{"ProjectNumber", "HostProjectNumber", "APIKey"}, nil)
}

// Implement GoStringer interface for the GCNVNASStorageDriverConfig driver
func (d GCNVNASStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *GCNVNASStorageDriverConfig) InjectSecrets(secretMap map[string]string) error {
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
func (d *GCNVNASStorageDriverConfig) ExtractSecrets() map[string]string {
	secretMap := make(map[string]string)

	secretMap["Private_Key"] = d.APIKey.PrivateKey
	secretMap["Private_Key_ID"] = d.APIKey.PrivateKeyID

	return secretMap
}

// RemoveSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *GCNVNASStorageDriverConfig) ResetSecrets() {
	d.APIKey.PrivateKey = ""
	d.APIKey.PrivateKeyID = ""
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *GCNVNASStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.APIKey.PrivateKey = secretName
	d.APIKey.PrivateKeyID = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *GCNVNASStorageDriverConfig) GetAndHideSensitive(secretName string) map[string]string {
	secretMap := d.ExtractSecrets()
	d.HideSensitiveWithSecretName(secretName)

	return secretMap
}

// CheckForCRDControllerForbiddenAttributes checks config for the keys forbidden by CRD controller and returns them
func (d GCNVNASStorageDriverConfig) CheckForCRDControllerForbiddenAttributes() []string {
	return checkMapContainsAttributes(d.ExtractSecrets())
}

func (d GCNVNASStorageDriverConfig) SpecOnlyValidation() error {
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
	// Dummy field for unit tests
	VolumeAccess string `json:"volumeAccess"`
	FakeStorageDriverPool
}

// Implement Stringer interface for the FakeStorageDriverConfig driver
func (d FakeStorageDriverConfig) String() string {
	return utils.ToStringRedacted(&d, []string{"Username", "Password"}, nil)
}

// Implement GoStringer interface for the FakeStorageDriverConfig driver
func (d FakeStorageDriverConfig) GoString() string {
	return d.String()
}

// InjectSecrets function replaces sensitive fields in the config with the field values in the map
func (d *FakeStorageDriverConfig) InjectSecrets(_ map[string]string) error {
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

// RemoveSecrets function removes sensitive fields it contains (credentials, etc.)
func (d *FakeStorageDriverConfig) ResetSecrets() {
	d.Username = ""
	d.Password = ""
}

// HideSensitiveWithSecretName function replaces sensitive fields it contains (credentials, etc.),
// with secretName.
func (d *FakeStorageDriverConfig) HideSensitiveWithSecretName(secretName string) {
	d.Username = secretName
	d.Password = secretName
}

// GetAndHideSensitive function builds a map of any sensitive fields it contains (credentials, etc.),
// replaces those fields with secretName and returns the the map.
func (d *FakeStorageDriverConfig) GetAndHideSensitive(_ string) map[string]string {
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

	for k := range credentials {
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

	allowedCredentialTypes := []string{
		string(CredentialStoreK8sSecret),
		string(CredentialStoreAWSARN),
	}

	if !utils.SliceContainsString(allowedCredentialTypes, secretStore) {
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
