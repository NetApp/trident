// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"time"

	"github.com/netapp/trident/storage"
)

const (
	StateAccepted  = "Accepted"
	StateCreating  = "Creating"
	StateAvailable = "Succeeded"
	StateDeleting  = "Deleting"
	StateDeleted   = "NoSuchState"
	StateMoving    = "Moving" // Currently unused by ANF
	StateError     = "Failed"
	StateReverting = "Reverting"

	ProtocolTypeNFSPrefix = "NFSv"
	ProtocolTypeNFSv3     = ProtocolTypeNFSPrefix + "3"
	ProtocolTypeNFSv41    = ProtocolTypeNFSPrefix + "4.1"
	ProtocolTypeCIFS      = "CIFS"

	MountOptionKerberos5  = "sec=krb5"
	MountOptionKerberos5I = "sec=krb5i"
	MountOptionKerberos5P = "sec=krb5p"

	ServiceLevelStandard = "Standard"
	ServiceLevelPremium  = "Premium"
	ServiceLevelUltra    = "Ultra"

	FeatureUnixPermissions = "ANFUnixPermissions"

	NetworkFeaturesBasic    = "Basic"
	NetworkFeaturesStandard = "Standard"

	EncryptionKeyNetApp = "Microsoft.NetApp"
	EncryptionKeyVault  = "Microsoft.KeyVault"

	QOSAuto   = "Auto"
	QOSManual = "Manual"
)

// AzureResources is the toplevel cache for the set of things we discover about our Azure environment.
type AzureResources struct {
	ResourceGroups    []*ResourceGroup
	ResourceGroupMap  map[string]*ResourceGroup
	NetAppAccountMap  map[string]*NetAppAccount
	CapacityPoolMap   map[string]*CapacityPool
	VirtualNetworkMap map[string]*VirtualNetwork
	SubnetMap         map[string]*Subnet
	StoragePoolMap    map[string]storage.Pool
	Features          map[string]bool
	lastUpdateTime    time.Time
}

// ResourceGroup records details of a discovered Azure ResourceGroup.
type ResourceGroup struct {
	Name            string
	NetAppAccounts  []*NetAppAccount
	VirtualNetworks []*VirtualNetwork
}

// NetAppAccount records details of a discovered ANF NetAppAccount.
type NetAppAccount struct {
	ID            string
	ResourceGroup string
	Name          string
	FullName      string
	Location      string
	Type          string
	CapacityPools []*CapacityPool
}

// VirtualNetwork records details of a discovered Azure Virtual Network.
type VirtualNetwork struct {
	ID            string
	ResourceGroup string
	Name          string
	FullName      string
	Location      string
	Type          string
	Subnets       []*Subnet
}

// Subnet records details of a discovered Azure Subnet.
type Subnet struct {
	ID             string
	ResourceGroup  string
	VirtualNetwork string
	Name           string
	FullName       string
	Location       string
	Type           string
}

// CapacityPool records details of a discovered Azure Subnet.
type CapacityPool struct {
	ID                string
	ResourceGroup     string
	NetAppAccount     string
	Name              string
	FullName          string
	Location          string
	Type              string
	PoolID            string
	ServiceLevel      string
	ProvisioningState string
	QosType           string
}

// FileSystem records details of a discovered Azure Subnet.
type FileSystem struct {
	ID                 string
	ResourceGroup      string
	NetAppAccount      string
	CapacityPool       string
	Name               string
	FullName           string
	Location           string
	Type               string
	ExportPolicy       ExportPolicy
	Labels             map[string]string
	FileSystemID       string
	ProvisioningState  string
	CreationToken      string
	ProtocolTypes      []string
	QuotaInBytes       int64
	ServiceLevel       string
	SnapshotDirectory  bool
	UsedBytes          int
	SubnetID           string
	UnixPermissions    string
	MountTargets       []MountTarget
	NetworkFeatures    string
	KerberosEnabled    bool
	KeyVaultEndpointID string
	Zones              []string
	MaxThroughput      float32
}

// FilesystemCreateRequest embodies all the details of a volume to be created.
type FilesystemCreateRequest struct {
	ResourceGroup      string
	NetAppAccount      string
	CapacityPool       string
	Name               string
	SubnetID           string
	CreationToken      string
	ExportPolicy       ExportPolicy
	Labels             map[string]string
	ProtocolTypes      []string
	QuotaInBytes       int64
	SnapshotDirectory  bool
	SnapshotID         string
	UnixPermissions    string
	NetworkFeatures    string
	KerberosEnabled    bool
	KeyVaultEndpointID string
	Zone               string
	MaxThroughput      *float32
}

// ExportPolicy records details of a discovered Azure volume export policy.
type ExportPolicy struct {
	Rules []ExportRule
}

// ExportRule records details of a discovered Azure volume export policy rule.
type ExportRule struct {
	AllowedClients      string
	Cifs                bool
	Nfsv3               bool
	Nfsv41              bool
	RuleIndex           int32
	UnixReadOnly        bool
	UnixReadWrite       bool
	Kerberos5ReadOnly   bool
	Kerberos5ReadWrite  bool
	Kerberos5IReadOnly  bool
	Kerberos5IReadWrite bool
	Kerberos5PReadOnly  bool
	Kerberos5PReadWrite bool
}

// MountTarget records details of a discovered Azure volume mount target.
type MountTarget struct {
	MountTargetID string
	FileSystemID  string
	IPAddress     string
	ServerFqdn    string
}

// Snapshot records details of a discovered Azure snapshot.
type Snapshot struct {
	ID                string
	ResourceGroup     string
	NetAppAccount     string
	CapacityPool      string
	Volume            string
	Name              string
	FullName          string
	Location          string
	Type              string
	Created           time.Time
	SnapshotID        string
	ProvisioningState string
}
