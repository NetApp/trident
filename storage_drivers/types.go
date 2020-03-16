// Copyright 2020 NetApp, Inc. All Rights Reserved.
package storagedrivers

import (
	"encoding/json"
	"fmt"
	"strings"

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

	EseriesStorageDriverPool
	Storage []EseriesStorageDriverPool `json:"storage"`
}

type EseriesStorageDriverPool struct {
	Labels                             map[string]string `json:"labels"`
	Region                             string            `json:"region"`
	Zone                               string            `json:"zone"`
	EseriesStorageDriverConfigDefaults `json:"defaults"`
}

type EseriesStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
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
	EmptyFlexvolDeferredDeletePeriod string   `json:"emptyFlexvolDeferredDeletePeriod"` // in seconds, default to 28800
	NfsMountOptions                  string   `json:"nfsMountOptions"`
	LimitAggregateUsage              string   `json:"limitAggregateUsage"`
	AutoExportPolicy                 bool     `json:"autoExportPolicy"`
	AutoExportCIDRs                  []string `json:"autoExportCIDRs"`
	OntapStorageDriverPool
	Storage []OntapStorageDriverPool `json:"storage"`
}

type OntapStorageDriverPool struct {
	Labels                           map[string]string `json:"labels"`
	Region                           string            `json:"region"`
	Zone                             string            `json:"zone"`
	OntapStorageDriverConfigDefaults `json:"defaults"`
}

type OntapStorageDriverConfigDefaults struct {
	SpaceAllocation string `json:"spaceAllocation"`
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
	TieringPolicy   string `json:"tieringPolicy"`
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
	Labels                               map[string]string `json:"labels"`
	Region                               string            `json:"region"`
	Zone                                 string            `json:"zone"`
	Type                                 string            `json:"type"`
	SolidfireStorageDriverConfigDefaults `json:"defaults"`
}

type SolidfireStorageDriverConfigDefaults struct {
	CommonStorageDriverConfigDefaults
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
	Labels                            map[string]string `json:"labels"`
	Region                            string            `json:"region"`
	Zone                              string            `json:"zone"`
	ServiceLevel                      string            `json:"serviceLevel"`
	AWSNFSStorageDriverConfigDefaults `json:"defaults"`
}

type AWSNFSStorageDriverConfigDefaults struct {
	ExportRule      string `json:"exportRule"`
	SnapshotDir     string `json:"snapshotDir"`
	SnapshotReserve string `json:"snapshotReserve"`
	CommonStorageDriverConfigDefaults
}

type AzureNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	SubscriptionID  string `json:"subscriptionID"`
	TenantID        string `json:"tenantID"`
	ClientID        string `json:"clientID"`
	ClientSecret    string `json:"clientSecret"`
	NfsMountOptions string `json:"nfsMountOptions"`
	AzureNFSStorageDriverPool
	Storage []AzureNFSStorageDriverPool `json:"storage"`
}

// Note that 'Region' and 'Zone' are internal specifiers, not related to Azure's
// 'Location' field.
type AzureNFSStorageDriverPool struct {
	Labels                              map[string]string `json:"labels"`
	Region                              string            `json:"region"`
	Zone                                string            `json:"zone"`
	Location                            string            `json:"location"`
	ServiceLevel                        string            `json:"serviceLevel"`
	VirtualNetwork                      string            `json:"virtualNetwork"`
	Subnet                              string            `json:"subnet"`
	AzureNFSStorageDriverConfigDefaults `json:"defaults"`
}

type AzureNFSStorageDriverConfigDefaults struct {
	ExportRule string `json:"exportRule"`
	CommonStorageDriverConfigDefaults
}

type GCPNFSStorageDriverConfig struct {
	*CommonStorageDriverConfig
	ProjectNumber   string        `json:"projectNumber"`
	APIKey          GCPPrivateKey `json:"apiKey"`
	APIRegion       string        `json:"apiRegion"`
	ProxyURL        string        `json:"proxyURL"`
	NfsMountOptions string        `json:"nfsMountOptions"`
	GCPNFSStorageDriverPool
	Storage []GCPNFSStorageDriverPool `json:"storage"`
}

type GCPNFSStorageDriverPool struct {
	Labels                            map[string]string `json:"labels"`
	Region                            string            `json:"region"`
	Zone                              string            `json:"zone"`
	ServiceLevel                      string            `json:"serviceLevel"`
	Network                           string            `json:"network"`
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

type SnapshotsNotSupportedError struct {
	message string
}

func (e *SnapshotsNotSupportedError) Error() string { return e.message }

func NewSnapshotsNotSupportedError(backendType string) error {
	return &SnapshotsNotSupportedError{
		message: fmt.Sprintf("snapshots are not supported by backend type %s", backendType),
	}
}
