// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"sync"
	"time"

	"github.com/netapp/trident/pkg/collection"
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
	resourceGroups    []*ResourceGroup
	resourceGroupMap  *collection.ImmutableMap[string, *ResourceGroup]
	netAppAccountMap  *collection.ImmutableMap[string, *NetAppAccount]
	capacityPoolMap   *collection.ImmutableMap[string, *CapacityPool]
	virtualNetworkMap *collection.ImmutableMap[string, *VirtualNetwork]
	subnetMap         *collection.ImmutableMap[string, *Subnet]
	storagePools      *collection.ImmutableMap[string, storage.Pool]
	features          *collection.ImmutableMap[string, bool]
	lastUpdateTime    time.Time
	m                 sync.Mutex
}

// azureResourceUpdater swaps in a full discovery snapshot; caller must hold AzureResources.m.
type azureResourceUpdater func(
	updateTime time.Time,
	resourceGroups []*ResourceGroup,
	resourceGroupMap map[string]*ResourceGroup,
	netAppAccountMap map[string]*NetAppAccount,
	capacityPoolMap map[string]*CapacityPool,
	virtualNetworkMap map[string]*VirtualNetwork,
	subnetMap map[string]*Subnet,
)

func newAzureResources() *AzureResources {
	return &AzureResources{
		resourceGroupMap:  collection.NewImmutableMap[string, *ResourceGroup](nil),
		netAppAccountMap:  collection.NewImmutableMap[string, *NetAppAccount](nil),
		capacityPoolMap:   collection.NewImmutableMap[string, *CapacityPool](nil),
		virtualNetworkMap: collection.NewImmutableMap[string, *VirtualNetwork](nil),
		subnetMap:         collection.NewImmutableMap[string, *Subnet](nil),
		storagePools:      collection.NewImmutableMap[string, storage.Pool](nil),
		features:          collection.NewImmutableMap[string, bool](nil),
	}
}

func (r *AzureResources) newDiscoveryUpdater() azureResourceUpdater {
	return func(
		updateTime time.Time,
		resourceGroups []*ResourceGroup,
		resourceGroupMap map[string]*ResourceGroup,
		netAppAccountMap map[string]*NetAppAccount,
		capacityPoolMap map[string]*CapacityPool,
		virtualNetworkMap map[string]*VirtualNetwork,
		subnetMap map[string]*Subnet,
	) {
		r.resourceGroups = resourceGroups
		r.resourceGroupMap = collection.NewImmutableMap(resourceGroupMap)
		r.netAppAccountMap = collection.NewImmutableMap(netAppAccountMap)
		r.capacityPoolMap = collection.NewImmutableMap(capacityPoolMap)
		r.virtualNetworkMap = collection.NewImmutableMap(virtualNetworkMap)
		r.subnetMap = collection.NewImmutableMap(subnetMap)
		r.lastUpdateTime = updateTime
	}
}

// LockAndCheckStale locks the resources and returns true if the cache is stale based on maxAge.
// The caller must call the returned unlock function when done. When stale is false, updater is nil.
func (r *AzureResources) LockAndCheckStale(maxAge time.Duration) (stale bool, updater azureResourceUpdater, unlock func()) {
	r.m.Lock()
	if time.Since(r.lastUpdateTime) <= maxAge {
		return false, nil, func() { r.m.Unlock() }
	}
	return true, r.newDiscoveryUpdater(), func() { r.m.Unlock() }
}

// LockForDiscover locks and returns an updater for a full discovery refresh (ignores cache age).
func (r *AzureResources) LockForDiscover() (updater azureResourceUpdater, unlock func()) {
	r.m.Lock()
	return r.newDiscoveryUpdater(), func() { r.m.Unlock() }
}

// SetStoragePools sets pools from the backend config; not thread-safe and should only be called during initialization.
func (r *AzureResources) SetStoragePools(storagePools map[string]storage.Pool) {
	r.storagePools = collection.NewImmutableMap(storagePools)
}

// GetStoragePools returns the pools defined in the backend config.
func (r *AzureResources) GetStoragePools() *collection.ImmutableMap[string, storage.Pool] {
	return r.storagePools
}

func (r *AzureResources) GetResourceGroups() []*ResourceGroup {
	r.m.Lock()
	defer r.m.Unlock()
	return r.resourceGroups
}

func (r *AzureResources) GetResourceGroupMap() *collection.ImmutableMap[string, *ResourceGroup] {
	r.m.Lock()
	defer r.m.Unlock()
	return r.resourceGroupMap
}

func (r *AzureResources) GetNetAppAccountMap() *collection.ImmutableMap[string, *NetAppAccount] {
	r.m.Lock()
	defer r.m.Unlock()
	return r.netAppAccountMap
}

func (r *AzureResources) GetCapacityPools() *collection.ImmutableMap[string, *CapacityPool] {
	r.m.Lock()
	defer r.m.Unlock()
	return r.capacityPoolMap
}

func (r *AzureResources) GetVirtualNetworkMap() *collection.ImmutableMap[string, *VirtualNetwork] {
	r.m.Lock()
	defer r.m.Unlock()
	return r.virtualNetworkMap
}

func (r *AzureResources) GetSubnetMap() *collection.ImmutableMap[string, *Subnet] {
	r.m.Lock()
	defer r.m.Unlock()
	return r.subnetMap
}

// SetFeatures replaces the feature flags map; caller should hold no locks — this method locks internally.
func (r *AzureResources) SetFeatures(featureMap map[string]bool) {
	r.m.Lock()
	defer r.m.Unlock()
	r.features = collection.NewImmutableMap(featureMap)
}

func (r *AzureResources) featuresSnapshot() map[string]bool {
	r.m.Lock()
	im := r.features
	r.m.Unlock()
	if im == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, im.Length())
	im.Range(func(k string, v bool) bool {
		out[k] = v
		return true
	})
	return out
}

func (r *AzureResources) hasFeature(name string) bool {
	r.m.Lock()
	im := r.features
	r.m.Unlock()
	if im == nil {
		return false
	}
	v, ok := im.GetOk(name)
	return ok && v
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
