// Copyright 2021 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Azure NetApp Files SDK
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/netapp/mgmt/2021-10-01/netapp"
	"github.com/Azure/azure-sdk-for-go/services/resourcegraph/mgmt/2021-03-01/resourcegraph"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2021-07-01/features"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	azauth "github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

const (
	VolumeCreateTimeout        = 10 * time.Second
	SnapshotTimeout            = 240 * time.Second // Snapshotter sidecar has a timeout of 5 minutes.  Stay under that!
	DefaultTimeout             = 120 * time.Second
	MaxLabelLength             = 256
	DefaultSDKTimeout          = 30 * time.Second
	DefaultSubvolumeSDKTimeout = 7 * time.Second // TODO: Replace custom retries with Autorest's built-in retries
	CorrelationIDHeader        = "X-Ms-Correlation-Request-Id"
	SubvolumeNameSeparator     = "-file-"
)

var (
	capacityPoolIDRegex = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)$`)
	volumeIDRegex       = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)$`)
	volumeNameRegex     = regexp.MustCompile(`[/]?(?P<resourceGroup>[^/]+)/(?P<netappAccount>[^/]+)/(?P<capacityPool>[^/]+)/(?P<volume>[^/]+)?[/]?$`)
	snapshotIDRegex     = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)/snapshots/(?P<snapshot>[^/]+)$`)
	subvolumeIDRegex    = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)/subvolumes/(?P<subvolume>[^/]+)$`)
	subnetIDRegex       = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/virtualNetworks/(?P<virtualNetwork>[^/]+)/subnets/(?P<subnet>[^/]+)$`)
)

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	// Azure API authentication parameters
	SubscriptionID string
	TenantID       string
	ClientID       string
	ClientSecret   string
	Location       string

	// Options
	DebugTraceFlags map[string]bool
	SDKTimeout      time.Duration // Timeout applied to all calls to the Azure SDK
	MaxCacheAge     time.Duration // The oldest data we should expect in the cached resources
}

// AzureClient holds operational Azure SDK objects.
type AzureClient struct {
	AuthConfig       azauth.ClientCredentialsConfig
	FeaturesClient   features.Client
	GraphClient      resourcegraph.BaseClient
	VolumesClient    netapp.VolumesClient
	SnapshotsClient  netapp.SnapshotsClient
	SubvolumesClient netapp.SubvolumesClient
	AzureResources
}

// Client encapsulates connection details.
type Client struct {
	config    *ClientConfig
	sdkClient *AzureClient
}

// NewDriver is a factory method for creating a new SDK interface.
func NewDriver(config ClientConfig) (Azure, error) {
	var err error

	// Ensure we got a location
	if config.Location == "" {
		return nil, errors.New("location must be specified in the config")
	}

	sdkClient := &AzureClient{
		AuthConfig:       azauth.NewClientCredentialsConfig(config.ClientID, config.ClientSecret, config.TenantID),
		FeaturesClient:   features.NewClient(config.SubscriptionID),
		GraphClient:      resourcegraph.New(),
		VolumesClient:    netapp.NewVolumesClient(config.SubscriptionID),
		SnapshotsClient:  netapp.NewSnapshotsClient(config.SubscriptionID),
		SubvolumesClient: netapp.NewSubvolumesClient(config.SubscriptionID),
	}

	// Set authorization endpoints
	sdkClient.AuthConfig.AADEndpoint = azure.PublicCloud.ActiveDirectoryEndpoint
	sdkClient.AuthConfig.Resource = azure.PublicCloud.ResourceManagerEndpoint

	// Plumb the authorization through to sub-clients.
	if sdkClient.FeaturesClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.GraphClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.VolumesClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.SnapshotsClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.SubvolumesClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}

	return Client{
		config:    &config,
		sdkClient: sdkClient,
	}, nil
}

// Init runs startup logic after allocating the driver resources.
func (c Client) Init(ctx context.Context, pools map[string]storage.Pool) error {
	// Map vpools to backend
	c.registerStoragePools(pools)

	// Find out what we have to work with in Azure
	return c.RefreshAzureResources(ctx)
}

// RegisterStoragePool makes a note of pools defined by the driver for later mapping.
func (c Client) registerStoragePools(sPools map[string]storage.Pool) {
	c.sdkClient.AzureResources.StoragePoolMap = make(map[string]storage.Pool)

	for _, sPool := range sPools {
		c.sdkClient.AzureResources.StoragePoolMap[sPool.Name()] = sPool
	}
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to create & parse Azure resource IDs and names
// ///////////////////////////////////////////////////////////////////////////////

// CreateVirtualNetworkID creates the Azure-style ID for a virtual network.
func CreateVirtualNetworkID(subscriptionID, resourceGroup, virtualNetwork string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s",
		subscriptionID, resourceGroup, virtualNetwork)
}

// CreateVirtualNetworkFullName creates the fully qualified name for a virtual network.
func CreateVirtualNetworkFullName(resourceGroup, virtualNetwork string) string {
	return fmt.Sprintf("%s/%s", resourceGroup, virtualNetwork)
}

// CreateSubnetID creates the Azure-style ID for a subnet.
func CreateSubnetID(subscriptionID, resourceGroup, vNet, subnet string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
		subscriptionID, resourceGroup, vNet, subnet)
}

// CreateSubnetFullName creates the fully qualified name for a subnet.
func CreateSubnetFullName(resourceGroup, virtualNetwork, subnet string) string {
	return fmt.Sprintf("%s/%s/%s", resourceGroup, virtualNetwork, subnet)
}

// ParseSubnetID parses the Azure-style ID for a subnet.
func ParseSubnetID(
	subnetID string,
) (subscriptionID, resourceGroup, provider, virtualNetwork, subnet string, err error) {
	match := subnetIDRegex.FindStringSubmatch(subnetID)

	if match == nil {
		err = fmt.Errorf("subnet ID %s is invalid", subnetID)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range subnetIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	subscriptionID = paramsMap["subscriptionID"]
	resourceGroup = paramsMap["resourceGroup"]
	provider = paramsMap["provider"]
	virtualNetwork = paramsMap["virtualNetwork"]
	subnet = paramsMap["subnet"]

	return
}

// CreateNetappAccountID creates the Azure-style ID for a netapp account.
func CreateNetappAccountID(subscriptionID, resourceGroup, netappAccount string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.NetApp/netAppAccounts/%s",
		subscriptionID, resourceGroup, netappAccount)
}

// CreateNetappAccountFullName creates the fully qualified name for a netapp account.
func CreateNetappAccountFullName(resourceGroup, netappAccount string) string {
	return fmt.Sprintf("%s/%s", resourceGroup, netappAccount)
}

// ParseCapacityPoolID parses the Azure-style ID for a capacity pool.
func ParseCapacityPoolID(
	capacityPoolID string,
) (subscriptionID, resourceGroup, provider, netappAccount, capacityPool string, err error) {
	match := capacityPoolIDRegex.FindStringSubmatch(capacityPoolID)

	if match == nil {
		err = fmt.Errorf("capacity pool ID %s is invalid", capacityPoolID)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range capacityPoolIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	subscriptionID = paramsMap["subscriptionID"]
	resourceGroup = paramsMap["resourceGroup"]
	provider = paramsMap["provider"]
	netappAccount = paramsMap["netappAccount"]
	capacityPool = paramsMap["capacityPool"]

	return
}

// CreateCapacityPoolFullName creates the fully qualified name for a capacity pool.
func CreateCapacityPoolFullName(resourceGroup, netappAccount, capacityPool string) string {
	return fmt.Sprintf("%s/%s/%s", resourceGroup, netappAccount, capacityPool)
}

// CreateVolumeID creates the Azure-style ID for a volume.
func CreateVolumeID(subscriptionID, resourceGroup, netappAccount, capacityPool, volume string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.NetApp/netAppAccounts/%s/capacityPools/%s/volumes/%s",
		subscriptionID, resourceGroup, netappAccount, capacityPool, volume)
}

// CreateVolumeFullName creates the fully qualified name for a volume.
func CreateVolumeFullName(resourceGroup, netappAccount, capacityPool, volume string) string {
	return fmt.Sprintf("%s/%s/%s/%s", resourceGroup, netappAccount, capacityPool, volume)
}

// ParseVolumeID parses the Azure-style ID for a volume.
func ParseVolumeID(
	volumeID string,
) (subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume string, err error) {
	match := volumeIDRegex.FindStringSubmatch(volumeID)

	if match == nil {
		err = fmt.Errorf("volume ID %s is invalid", volumeID)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range volumeIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	subscriptionID = paramsMap["subscriptionID"]
	resourceGroup = paramsMap["resourceGroup"]
	provider = paramsMap["provider"]
	netappAccount = paramsMap["netappAccount"]
	capacityPool = paramsMap["capacityPool"]
	volume = paramsMap["volume"]

	return
}

// ParseVolumeName parses the Azure-style Name for a volume.
func ParseVolumeName(volumeName string) (resourceGroup, netappAccount, capacityPool, volume string, err error) {
	match := volumeNameRegex.FindStringSubmatch(volumeName)

	paramsMap := make(map[string]string)
	for i, name := range volumeNameRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	var ok bool

	if resourceGroup, ok = paramsMap["resourceGroup"]; !ok {
		err = fmt.Errorf("volume name %s does not contain a resource group", volumeName)
		return
	}
	if netappAccount, ok = paramsMap["netappAccount"]; !ok {
		err = fmt.Errorf("volume name %s does not contain a netapp account", volumeName)
		return
	}
	if capacityPool, ok = paramsMap["capacityPool"]; !ok {
		err = fmt.Errorf("volume name %s does not contain a capacity pool", volumeName)
		return
	}
	if volume, ok = paramsMap["volume"]; !ok {
		err = fmt.Errorf("volume name %s does not contain a volume", volumeName)
		return
	}

	return
}

// CreateSnapshotID creates the Azure-style ID for a snapshot.
func CreateSnapshotID(
	subscriptionID, resourceGroup, netappAccount, capacityPool, volume, snapshot string,
) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.NetApp/netAppAccounts/%s/capacityPools/%s/volumes/%s/snapshots/%s",
		subscriptionID, resourceGroup, netappAccount, capacityPool, volume, snapshot)
}

// CreateSnapshotFullName creates the fully qualified name for a snapshot.
func CreateSnapshotFullName(resourceGroup, netappAccount, capacityPool, volume, snapshot string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", resourceGroup, netappAccount, capacityPool, volume, snapshot)
}

// ParseSnapshotID parses the Azure-style ID for a snapshot.
func ParseSnapshotID(
	snapshotID string,
) (subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume, snapshot string, err error) {
	match := snapshotIDRegex.FindStringSubmatch(snapshotID)

	if match == nil {
		err = fmt.Errorf("snapshot ID %s is invalid", snapshotID)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range snapshotIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	subscriptionID = paramsMap["subscriptionID"]
	resourceGroup = paramsMap["resourceGroup"]
	provider = paramsMap["provider"]
	netappAccount = paramsMap["netappAccount"]
	capacityPool = paramsMap["capacityPool"]
	volume = paramsMap["volume"]
	snapshot = paramsMap["snapshot"]

	return
}

// CreateSubvolumeID creates the Azure-style ID for a subvolume.
func CreateSubvolumeID(
	subscriptionID, resourceGroup, netappAccount, capacityPool, volume, subvolume string,
) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft."+
		"NetApp/netAppAccounts/%s/capacityPools/%s/volumes/%s/subvolumes/%s",
		subscriptionID, resourceGroup, netappAccount, capacityPool, volume, subvolume)
}

// CreateSubvolumeFullName creates the fully qualified name for a subvolume.
func CreateSubvolumeFullName(resourceGroup, netappAccount, capacityPool, volume, subvolume string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", resourceGroup, netappAccount, capacityPool, volume, subvolume)
}

// ParseSubvolumeID parses the Azure-style ID for a subvolume.
func ParseSubvolumeID(
	subvolumeID string,
) (subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume, subvolume string, err error) {
	match := subvolumeIDRegex.FindStringSubmatch(subvolumeID)

	paramsMap := make(map[string]string)
	for i, name := range subvolumeIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	var ok bool

	if subscriptionID, ok = paramsMap["subscriptionID"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a subscription ID", subvolumeID)
		return
	}
	if resourceGroup, ok = paramsMap["resourceGroup"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a resource group", subvolumeID)
		return
	}
	if provider, ok = paramsMap["provider"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a provider", subvolumeID)
		return
	}
	if netappAccount, ok = paramsMap["netappAccount"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a netapp account", subvolumeID)
		return
	}
	if capacityPool, ok = paramsMap["capacityPool"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a capacity pool", subvolumeID)
		return
	}
	if volume, ok = paramsMap["volume"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a volume", subvolumeID)
		return
	}
	if subvolume, ok = paramsMap["subvolume"]; !ok {
		err = fmt.Errorf("subvolume ID %s does not contain a subvolume", subvolumeID)
		return
	}

	return
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to convert between ANF SDK & internal volume structs
// ///////////////////////////////////////////////////////////////////////////////

// exportPolicyExport turns an internal ExportPolicy into something consumable by the SDK.
func exportPolicyExport(exportPolicy *ExportPolicy) *netapp.VolumePropertiesExportPolicy {
	anfRules := make([]netapp.ExportPolicyRule, 0)

	for _, rule := range exportPolicy.Rules {

		ruleIndex := rule.RuleIndex
		unixReadOnly := rule.UnixReadOnly
		unixReadWrite := rule.UnixReadWrite
		cifs := rule.Cifs
		nfsv3 := rule.Nfsv3
		nfsv41 := rule.Nfsv41
		allowedClients := rule.AllowedClients

		anfRule := netapp.ExportPolicyRule{
			RuleIndex:      &ruleIndex,
			UnixReadOnly:   &unixReadOnly,
			UnixReadWrite:  &unixReadWrite,
			Cifs:           &cifs,
			Nfsv3:          &nfsv3,
			Nfsv41:         &nfsv41,
			AllowedClients: &allowedClients,
		}

		anfRules = append(anfRules, anfRule)
	}

	return &netapp.VolumePropertiesExportPolicy{
		Rules: &anfRules,
	}
}

// exportPolicyImport turns an SDK ExportPolicy into an internal one.
func exportPolicyImport(anfExportPolicy *netapp.VolumePropertiesExportPolicy) *ExportPolicy {
	rules := make([]ExportRule, 0)

	if anfExportPolicy == nil || *anfExportPolicy.Rules == nil {
		return &ExportPolicy{Rules: rules}
	}

	for _, anfRule := range *anfExportPolicy.Rules {

		rule := ExportRule{
			RuleIndex:      DerefInt32(anfRule.RuleIndex),
			UnixReadOnly:   DerefBool(anfRule.UnixReadOnly),
			UnixReadWrite:  DerefBool(anfRule.UnixReadWrite),
			Cifs:           DerefBool(anfRule.Cifs),
			Nfsv3:          DerefBool(anfRule.Nfsv3),
			Nfsv41:         DerefBool(anfRule.Nfsv41),
			AllowedClients: DerefString(anfRule.AllowedClients),
		}

		rules = append(rules, rule)
	}

	return &ExportPolicy{Rules: rules}
}

// newFileSystemFromVolume creates a new internal FileSystem struct from an SDK volume.
func (c Client) newFileSystemFromVolume(ctx context.Context, vol *netapp.Volume) (*FileSystem, error) {
	if vol.ID == nil {
		return nil, errors.New("volume ID may not be nil")
	}

	_, resourceGroup, _, netappAccount, cPoolName, name, err := ParseVolumeID(*vol.ID)
	if err != nil {
		return nil, err
	}

	cPoolFullName := CreateCapacityPoolFullName(resourceGroup, netappAccount, cPoolName)
	cPool := c.capacityPool(cPoolFullName)
	if cPool == nil {
		return nil, fmt.Errorf("could not find capacity pool %s", cPoolFullName)
	}

	if vol.Location == nil {
		return nil, fmt.Errorf("volume %s has no location", *vol.Name)
	}

	if vol.VolumeProperties == nil {
		return nil, fmt.Errorf("volume %s has no properties", *vol.Name)
	}

	return &FileSystem{
		ID:                DerefString(vol.ID),
		ResourceGroup:     resourceGroup,
		NetAppAccount:     netappAccount,
		CapacityPool:      cPoolName,
		Name:              name,
		FullName:          CreateVolumeFullName(resourceGroup, netappAccount, cPoolName, name),
		Location:          DerefString(vol.Location),
		Type:              DerefString(vol.Type),
		ExportPolicy:      *exportPolicyImport(vol.VolumeProperties.ExportPolicy),
		Labels:            c.getLabelsFromVolume(vol),
		FileSystemID:      DerefString(vol.VolumeProperties.FileSystemID),
		ProvisioningState: DerefString(vol.VolumeProperties.ProvisioningState),
		CreationToken:     DerefString(vol.VolumeProperties.CreationToken),
		ProtocolTypes:     DerefStringArray(vol.VolumeProperties.ProtocolTypes),
		QuotaInBytes:      DerefInt64(vol.VolumeProperties.UsageThreshold),
		ServiceLevel:      cPool.ServiceLevel,
		SnapshotDirectory: DerefBool(vol.SnapshotDirectoryVisible),
		SubnetID:          DerefString(vol.VolumeProperties.SubnetID),
		UnixPermissions:   DerefString(vol.VolumeProperties.UnixPermissions),
		MountTargets:      c.getMountTargetsFromVolume(ctx, vol),
		SubvolumesEnabled: c.getSubvolumesEnabledFromVolume(vol.EnableSubvolumes),
		NetworkFeatures:   string(vol.NetworkFeatures),
	}, nil
}

// getSubvolumesEnabledFromVolume extracts the SubvolumesEnabled from an SDK volume.
func (c Client) getSubvolumesEnabledFromVolume(value netapp.EnableSubvolumes) bool {
	if value != netapp.EnableSubvolumesEnabled {
		return false
	}
	return true
}

// getMountTargetsFromVolume extracts the mount targets from an SDK volume.
func (c Client) getMountTargetsFromVolume(ctx context.Context, vol *netapp.Volume) []MountTarget {
	mounts := make([]MountTarget, 0)

	if vol.MountTargets == nil {
		Logc(ctx).Tracef("Volume %s has nil MountTargetProperties.", *vol.Name)
		return mounts
	}

	for _, mtp := range *vol.MountTargets {

		mt := MountTarget{
			MountTargetID: DerefString(mtp.MountTargetID),
			FileSystemID:  DerefString(mtp.FileSystemID),
			IPAddress:     DerefString(mtp.IPAddress),
			SmbServerFqdn: DerefString(mtp.SmbServerFqdn),
		}

		mounts = append(mounts, mt)
	}

	return mounts
}

// getLabelsFromVolume extracts the tags from an SDK volume.
func (c Client) getLabelsFromVolume(vol *netapp.Volume) map[string]string {
	labels := make(map[string]string)

	for k, v := range vol.Tags {
		labels[k] = *v
	}

	return labels
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to retrieve ANF volumes
// ///////////////////////////////////////////////////////////////////////////////

// getVolumesFromPool gets a set of volumes belonging to a single capacity pool.  As pools can come and go
// in between cache updates, we ignore any 404 errors here.
func (c Client) getVolumesFromPool(ctx context.Context, cPool *CapacityPool) (*[]*FileSystem, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	var filesystems []*FileSystem

	volumeList, err := c.sdkClient.VolumesClient.List(sdkCtx, cPool.ResourceGroup, cPool.NetAppAccount, cPool.Name)
	if err != nil {

		logFields := log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.List",
			"capacityPool":  cPool.FullName,
		}

		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Warning("Capacity pool not found, ignoring.")
			return &filesystems, nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volumes from capacity pool.")
		return nil, err
	}

	if c.config.DebugTraceFlags["api"] {
		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationID(volumeList.Response().Response.Response),
			"API":           "VolumesClient.List",
			"capacityPool":  cPool.FullName,
		}).Debug("Read volumes from capacity pool.")
	}

	for ; volumeList.NotDone(); err = volumeList.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating volumes; %v", err)
		}

		volumes := volumeList.Values()

		for idx := range volumes {
			filesystem, fsErr := c.newFileSystemFromVolume(ctx, &volumes[idx])
			if fsErr != nil {
				Logc(ctx).WithError(fsErr).Errorf("Internal error creating filesystem.")
				return nil, fsErr
			}
			filesystems = append(filesystems, filesystem)
		}
	}

	return &filesystems, nil
}

// Volumes returns a list of all volumes.
func (c Client) Volumes(ctx context.Context) (*[]*FileSystem, error) {
	var filesystems []*FileSystem

	cPools := c.CapacityPools()

	for _, cPool := range *cPools {

		poolFilesystems, err := c.getVolumesFromPool(ctx, cPool)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Error fetching volumes from pool %s.", cPool.Name)
			return nil, err
		}
		filesystems = append(filesystems, *poolFilesystems...)
	}

	return &filesystems, nil
}

// Volume uses a volume config record to fetch a volume by the most efficient means.
func (c Client) Volume(ctx context.Context, volConfig *storage.VolumeConfig) (*FileSystem, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return c.VolumeByID(ctx, volConfig.InternalID)
	}

	// Fall back to the creation token
	return c.VolumeByCreationToken(ctx, volConfig.InternalName)
}

// VolumeExists uses a volume config record to look for a Filesystem by the most efficient means.
func (c Client) VolumeExists(ctx context.Context, volConfig *storage.VolumeConfig) (bool, *FileSystem, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return c.VolumeExistsByID(ctx, volConfig.InternalID)
	}

	// Fall back to the creation token
	return c.VolumeExistsByCreationToken(ctx, volConfig.InternalName)
}

// VolumeByCreationToken fetches a Filesystem by its immutable creation token.  We can't query the SDK for
// volumes by creation token, so our only choice here is to get all volumes and return the one of interest.
// That is obviously very inefficient, so this method should be used only when absolutely necessary.
func (c Client) VolumeByCreationToken(ctx context.Context, creationToken string) (*FileSystem, error) {
	Logc(ctx).Tracef("Fetching volume by creation token %s.", creationToken)

	// SDK does not support searching by creation token. Or anything besides pool+name,
	// for that matter. Get all volumes and find it ourselves. This is far from ideal.
	filesystems, err := c.Volumes(ctx)
	if err != nil {
		return nil, err
	}

	matchingFilesystems := make([]*FileSystem, 0)

	for _, filesystem := range *filesystems {
		if filesystem.CreationToken == creationToken {
			matchingFilesystems = append(matchingFilesystems, filesystem)
		}
	}

	switch len(matchingFilesystems) {
	case 0:
		return nil, utils.NotFoundError(fmt.Sprintf("volume with creation token '%s' not found", creationToken))
	case 1:
		return matchingFilesystems[0], nil
	default:
		return nil, utils.NotFoundError(fmt.Sprintf("multiple volumes with creation token '%s' found", creationToken))
	}
}

// VolumeExistsByCreationToken checks whether a volume exists using its creation token as a key.
func (c Client) VolumeExistsByCreationToken(ctx context.Context, creationToken string) (bool, *FileSystem, error) {
	if filesystem, err := c.VolumeByCreationToken(ctx, creationToken); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	} else {
		return true, filesystem, nil
	}
}

// VolumeByID returns a Filesystem based on its Azure-style ID.
func (c Client) VolumeByID(ctx context.Context, id string) (*FileSystem, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	Logc(ctx).Tracef("Fetching volume by ID %s.", id)

	_, resourceGroup, _, netappAccount, cPoolName, volumeName, err := ParseVolumeID(id)
	if err != nil {
		return nil, err
	}

	volume, err := c.sdkClient.VolumesClient.Get(sdkCtx, resourceGroup, netappAccount, cPoolName, volumeName)
	if err != nil {

		logFields := log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.Get",
			"ID":            id,
		}

		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Debug("Volume not found.")
			return nil, utils.NotFoundError(fmt.Sprintf("volume with ID '%s' not found", id))
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(volume.Response.Response),
		"API":           "VolumesClient.Get",
		"ID":            id,
	}).Debug("Found volume by ID.")

	return c.newFileSystemFromVolume(ctx, &volume)
}

// VolumeExistsByID checks whether a volume exists using its creation token as a key.
func (c Client) VolumeExistsByID(ctx context.Context, id string) (bool, *FileSystem, error) {
	if filesystem, err := c.VolumeByID(ctx, id); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	} else {
		return true, filesystem, nil
	}
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to manage volumes
// ///////////////////////////////////////////////////////////////////////////////

// WaitForVolumeState watches for a desired volume state and returns when that state is achieved.
func (c Client) WaitForVolumeState(
	ctx context.Context, filesystem *FileSystem, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) (string, error) {
	volumeState := ""

	checkVolumeState := func() error {
		f, err := c.VolumeByID(ctx, filesystem.ID)
		if err != nil {

			// There is no 'Deleted' state in Azure -- the volume just vanishes.  If we failed to query
			// the volume info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if desiredState == StateDeleted && utils.IsNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for volume %s.", filesystem.Name)
				volumeState = StateDeleted
				return nil
			}

			return fmt.Errorf("could not get volume status; %v", err)
		}

		volumeState = f.ProvisioningState

		if f.ProvisioningState == desiredState {
			return nil
		}

		err = fmt.Errorf("volume state is %s, not %s", f.ProvisioningState, desiredState)

		// Return a permanent error to stop retrying if we reached one of the abort states
		if utils.SliceContainsString(abortStates, f.ProvisioningState) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for volume state.")
	}

	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 3 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if IsTerminalStateError(err) {
			Logc(ctx).WithError(err).Error("Volume reached terminal state.")
		} else {
			Logc(ctx).Errorf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return volumeState, nil
}

// CreateVolume creates a new volume.
func (c Client) CreateVolume(ctx context.Context, request *FilesystemCreateRequest) (*FileSystem, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	resourceGroup := request.ResourceGroup
	netappAccount := request.NetAppAccount
	cPoolName := request.CapacityPool

	// Get the capacity pool so we can determine location and service level
	cPoolFullName := CreateCapacityPoolFullName(resourceGroup, netappAccount, cPoolName)
	cPool, ok := c.sdkClient.AzureResources.CapacityPoolMap[cPoolFullName]
	if !ok {
		return nil, fmt.Errorf("unknown capacity pool %s", cPoolFullName)
	}

	volumeFullName := CreateVolumeFullName(resourceGroup, netappAccount, cPoolName, request.Name)

	// Location is required and is derived from the capacity pool
	location := cPool.Location

	tags := make(map[string]*string)
	for k, v := range request.Labels {
		tag := [1]string{v}
		tags[k] = &tag[0]
	}

	newVol := netapp.Volume{
		Location: &location,
		Name:     &request.Name,
		Tags:     tags,
		VolumeProperties: &netapp.VolumeProperties{
			CreationToken:            &request.CreationToken,
			ServiceLevel:             netapp.ServiceLevel(cPool.ServiceLevel),
			UsageThreshold:           &request.QuotaInBytes,
			ExportPolicy:             exportPolicyExport(&request.ExportPolicy),
			ProtocolTypes:            &request.ProtocolTypes,
			SubnetID:                 &request.SubnetID,
			SnapshotDirectoryVisible: &request.SnapshotDirectory,
			NetworkFeatures:          netapp.NetworkFeatures(request.NetworkFeatures),
		},
	}

	// Only set the snapshot ID if we are cloning
	if request.SnapshotID != "" {
		newVol.SnapshotID = &request.SnapshotID
	}

	// Only send unix permissions if specified, since it is not yet a GA feature
	if request.UnixPermissions != "" {
		newVol.VolumeProperties.UnixPermissions = &request.UnixPermissions
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"resourceGroup": resourceGroup,
		"netAppAccount": netappAccount,
		"capacityPool":  cPoolName,
		"subnetID":      request.SubnetID,
		"snapshotID":    request.SnapshotID,
		"snapshotDir":   request.SnapshotDirectory,
	}).Debug("Issuing create request.")

	future, err := c.sdkClient.VolumesClient.CreateOrUpdate(sdkCtx,
		newVol, resourceGroup, netappAccount, cPoolName, request.Name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.CreateOrUpdate",
			"volume":        volumeFullName,
			"creationToken": request.CreationToken,
		}).WithError(err).Error("Error creating volume.")

		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "VolumesClient.CreateOrUpdate",
		"volume":        volumeFullName,
		"creationToken": request.CreationToken,
	}).Info("Volume created.")

	// The volume doesn't exist yet, so forge the volume ID to enable conversion to a FileSystem struct
	newVolID := CreateVolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, cPoolName, request.Name)
	newVol.ID = &newVolID

	return c.newFileSystemFromVolume(ctx, &newVol)
}

// ModifyVolume updates attributes of a volume.
func (c Client) ModifyVolume(
	ctx context.Context, filesystem *FileSystem, labels map[string]string, unixPermissions *string,
) error {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	// Fetch the netapp.Volume to fill in the updated fields
	anfVolume, err := c.sdkClient.VolumesClient.Get(sdkCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.Get",
			"volume":        filesystem.FullName,
		}).WithError(err).Error("Error finding volume to modify.")

		return fmt.Errorf("couldn't get volume to modify; %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(anfVolume.Response.Response),
		"API":           "VolumesClient.Get",
		"volume":        filesystem.FullName,
	}).Debug("Found volume to modify.")

	tags := make(map[string]*string)

	if labels != nil {

		// Copy any existing tags first in order to make sure to preserve any custom tags that might've
		// been applied prior to a volume import
		for k, v := range anfVolume.Tags {
			tags[k] = v
		}

		// Now update the working copy with the incoming change
		for k, v := range labels {
			tag := [1]string{v}
			tags[k] = &tag[0]
		}

		anfVolume.Tags = tags
	}

	if unixPermissions != nil {
		anfVolume.UnixPermissions = unixPermissions
	}

	// Clear out ReadOnly and other fields that we don't want to change when merely relabeling.
	anfVolume.ServiceLevel = ""
	anfVolume.ProvisioningState = nil
	anfVolume.ExportPolicy = nil
	anfVolume.ProtocolTypes = nil
	anfVolume.MountTargets = nil
	anfVolume.ThroughputMibps = nil
	anfVolume.BaremetalTenantID = nil

	Logc(ctx).WithFields(log.Fields{
		"name":          anfVolume.Name,
		"creationToken": anfVolume.CreationToken,
	}).Debug("Modifying volume.")

	future, err := c.sdkClient.VolumesClient.CreateOrUpdate(sdkCtx,
		anfVolume, filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.CreateOrUpdate",
			"volume":        filesystem.FullName,
		}).WithError(err).Error("Error modifying volume.")

		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "VolumesClient.CreateOrUpdate",
		"volume":        filesystem.FullName,
	}).Debug("Volume modified.")

	return nil
}

// ResizeVolume sends a VolumePatch to update a volume's quota.
func (c Client) ResizeVolume(ctx context.Context, filesystem *FileSystem, newSizeBytes int64) error {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	patch := netapp.VolumePatch{
		ID:       &filesystem.ID,
		Location: &filesystem.Location,
		Name:     &filesystem.Name,
		VolumePatchProperties: &netapp.VolumePatchProperties{
			UsageThreshold: &newSizeBytes,
		},
	}

	future, err := c.sdkClient.VolumesClient.Update(sdkCtx,
		patch, filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.Update",
			"volume":        filesystem.FullName,
		}).WithError(err).Error("Error resizing volume.")

		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "VolumesClient.Update",
		"volume":        filesystem.FullName,
	}).Debug("Volume resized.")

	return nil
}

// DeleteVolume deletes a volume.
func (c Client) DeleteVolume(ctx context.Context, filesystem *FileSystem) error {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	future, err := c.sdkClient.VolumesClient.Delete(sdkCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, nil)
	if err != nil {

		logFields := log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "VolumesClient.Delete",
			"volume":        filesystem.FullName,
		}

		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Info("Volume already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting volume.")
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "VolumesClient.Delete",
		"volume":        filesystem.FullName,
	}).Debug("Volume deleted.")

	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to retrieve and manage snapshots
// ///////////////////////////////////////////////////////////////////////////////

// newSnapshotFromANFSnapshot creates a new internal Snapshot struct from a netapp.Snapshot.
func (c Client) newSnapshotFromANFSnapshot(_ context.Context, anfSnapshot *netapp.Snapshot) (*Snapshot, error) {
	if anfSnapshot.ID == nil {
		return nil, errors.New("snapshot ID may not be nil")
	}

	_, resourceGroup, _, netappAccount, cPool, volume, snapshotName, err := ParseSnapshotID(*anfSnapshot.ID)
	if err != nil {
		return nil, err
	}

	if anfSnapshot.Location == nil {
		return nil, fmt.Errorf("snapshot %s has no location", *anfSnapshot.Name)
	}

	if anfSnapshot.SnapshotProperties == nil {
		return nil, fmt.Errorf("snapshot %s has no properties", *anfSnapshot.Name)
	}

	snapshot := Snapshot{
		ID:                DerefString(anfSnapshot.ID),
		ResourceGroup:     resourceGroup,
		NetAppAccount:     netappAccount,
		CapacityPool:      cPool,
		Volume:            volume,
		Name:              snapshotName,
		FullName:          CreateSnapshotFullName(resourceGroup, netappAccount, cPool, volume, snapshotName),
		Location:          DerefString(anfSnapshot.Location),
		Type:              DerefString(anfSnapshot.Type),
		SnapshotID:        DerefString(anfSnapshot.SnapshotID),
		ProvisioningState: DerefString(anfSnapshot.ProvisioningState),
	}

	if anfSnapshot.Created != nil {
		snapshot.Created = anfSnapshot.Created.ToTime()
	}

	return &snapshot, nil
}

// SnapshotsForVolume returns a list of snapshots on a volume.
func (c Client) SnapshotsForVolume(ctx context.Context, filesystem *FileSystem) (*[]*Snapshot, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	snapshotList, err := c.sdkClient.SnapshotsClient.List(sdkCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "SnapshotsClient.List",
			"volume":        filesystem.FullName,
		}).WithError(err).Error("Error fetching snapshots from volume.")

		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(snapshotList.Response.Response),
		"API":           "SnapshotsClient.List",
		"volume":        filesystem.FullName,
	}).Debug("Read snapshots from volume.")

	var snapshots []*Snapshot

	for idx := range *snapshotList.Value {
		anfSnapshot := (*snapshotList.Value)[idx]
		snapshot, snapErr := c.newSnapshotFromANFSnapshot(ctx, &anfSnapshot)
		if snapErr != nil {
			Logc(ctx).WithError(snapErr).Errorf("Internal error creating snapshot.")
			return nil, snapErr
		}

		snapshots = append(snapshots, snapshot)
	}

	return &snapshots, nil
}

// SnapshotForVolume fetches a specific snapshot on a volume by its name.
func (c Client) SnapshotForVolume(
	ctx context.Context, filesystem *FileSystem, snapshotName string,
) (*Snapshot, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	anfSnapshot, err := c.sdkClient.SnapshotsClient.Get(sdkCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, snapshotName)
	if err != nil {

		logFields := log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "SnapshotsClient.Get",
			"volume":        filesystem.FullName,
			"snapshot":      snapshotName,
		}

		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Debug("Snapshot not found.")
			return nil, utils.NotFoundError(fmt.Sprintf("snapshot %s not found", snapshotName))
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching snapshot.")
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(anfSnapshot.Response.Response),
		"API":           "SnapshotsClient.Get",
		"volume":        filesystem.FullName,
		"snapshot":      snapshotName,
	}).Debug("Found snapshot.")

	return c.newSnapshotFromANFSnapshot(ctx, &anfSnapshot)
}

// WaitForSnapshotState waits for a desired snapshot state and returns once that state is achieved.
func (c Client) WaitForSnapshotState(
	ctx context.Context, snapshot *Snapshot, filesystem *FileSystem, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) error {
	checkSnapshotState := func() error {
		s, err := c.SnapshotForVolume(ctx, filesystem, snapshot.Name)
		if err != nil {

			// There is no 'Deleted' state in Azure -- the snapshot just vanishes.  If we failed to query
			// the snapshot info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if desiredState == StateDeleted && utils.IsNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for snapshot %s.", snapshot.Name)
				return nil
			}
			return fmt.Errorf("could not get snapshot status; %v", err)
		}

		if s.ProvisioningState == desiredState {
			return nil
		}

		err = fmt.Errorf("snapshot state is %s, not %s", s.ProvisioningState, desiredState)

		// Return a permanent error to stop retrying if we reached one of the abort states
		if utils.SliceContainsString(abortStates, s.ProvisioningState) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for snapshot state.")
	}

	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 3 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for snapshot state.")

	if err := backoff.RetryNotify(checkSnapshotState, stateBackoff, stateNotify); err != nil {
		if IsTerminalStateError(err) {
			Logc(ctx).WithError(err).Error("Snapshot reached terminal state.")
		} else {
			Logc(ctx).Errorf("Snapshot state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debugf("Desired snapshot state reached.")

	return nil
}

// CreateSnapshot creates a new snapshot.
func (c Client) CreateSnapshot(ctx context.Context, filesystem *FileSystem, name string) (*Snapshot, error) {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	anfSnapshot := netapp.Snapshot{
		Location: &filesystem.Location,
		Name:     &name,
	}

	// Create the snapshot
	future, err := c.sdkClient.SnapshotsClient.Create(sdkCtx,
		anfSnapshot, filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, name)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "SnapshotsClient.Create",
			"volume":        filesystem.FullName,
			"snapshot":      name,
		}).WithError(err).Error("Error creating snapshot.")

		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "SnapshotsClient.Create",
		"volume":        filesystem.FullName,
		"snapshot":      name,
	}).Info("Snapshot created.")

	// The snapshot doesn't exist yet, so forge the snapshot ID to enable conversion to a Snapshot struct
	newSnapshotID := CreateSnapshotID(c.config.SubscriptionID, filesystem.ResourceGroup,
		filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, name)
	anfSnapshot.ID = &newSnapshotID

	anfSnapshot.SnapshotProperties = &netapp.SnapshotProperties{}

	return c.newSnapshotFromANFSnapshot(ctx, &anfSnapshot)
}

// DeleteSnapshot deletes a snapshot.
func (c Client) DeleteSnapshot(ctx context.Context, filesystem *FileSystem, snapshot *Snapshot) error {
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()

	future, err := c.sdkClient.SnapshotsClient.Delete(sdkCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, snapshot.Name)
	if err != nil {

		logFields := log.Fields{
			"correlationID": GetCorrelationIDFromError(err),
			"API":           "SnapshotsClient.Delete",
			"volume":        filesystem.FullName,
			"snapshot":      snapshot.Name,
		}

		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Info("Snapshot already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"correlationID": GetCorrelationID(future.Response()),
		"API":           "SnapshotsClient.Delete",
		"volume":        filesystem.FullName,
		"snapshot":      snapshot.Name,
	}).Debug("Snapshot deleted.")

	return nil
}

// newSubvolumeFromSubvolumeInfo creates a new internal Subvolume struct from an SDK SubvolumeInfo
func (c Client) newSubvolumeFromSubvolumeInfo(_ context.Context, subVol *netapp.SubvolumeInfo) (*Subvolume, error) {
	if subVol.ID == nil {
		return nil, errors.New("subvolume ID may not be nil")
	}

	_, resourceGroup, _, netappAccount, cPoolName, volumeName, subvolumeName, err := ParseSubvolumeID(*subVol.ID)
	if err != nil {
		return nil, err
	}

	cPoolFullName := CreateCapacityPoolFullName(resourceGroup, netappAccount, cPoolName)
	cPool := c.capacityPool(cPoolFullName)
	if cPool == nil {
		return nil, fmt.Errorf("could not find capacity pool %s", cPoolFullName)
	}

	if subVol.SubvolumeProperties == nil {
		return nil, fmt.Errorf("subvolume %s has no properties", *subVol.Name)
	}

	subvolume := Subvolume{
		ID:                DerefString(subVol.ID),
		ResourceGroup:     resourceGroup,
		NetAppAccount:     netappAccount,
		CapacityPool:      cPoolName,
		Volume:            volumeName,
		Name:              subvolumeName,
		FullName:          CreateSubvolumeFullName(resourceGroup, netappAccount, cPoolName, volumeName, subvolumeName),
		Type:              DerefString(subVol.Type),
		ProvisioningState: DerefString(subVol.SubvolumeProperties.ProvisioningState),
		Size:              DerefInt64(subVol.SubvolumeProperties.Size),
	}

	return &subvolume, nil
}

// updateSubvolumeFromSubvolumeModel updates an existing internal Subvolume struct from an SDK SubvolumeModel
func (c Client) updateSubvolumeFromSubvolumeModel(
	_ context.Context, original *Subvolume, subVolModel *netapp.SubvolumeModel,
) (*Subvolume, error) {
	if subVolModel.ID == nil {
		return nil, errors.New("subvolume ID may not be nil")
	}

	if subVolModel.SubvolumeModelProperties == nil {
		return nil, fmt.Errorf("subvolume model %s has no properties", *subVolModel.Name)
	}

	original.ProvisioningState = DerefString(subVolModel.SubvolumeModelProperties.ProvisioningState)
	original.Size = DerefInt64(subVolModel.SubvolumeModelProperties.Size)

	if subVolModel.SubvolumeModelProperties.CreationTimeStamp != nil {
		original.Created = subVolModel.SubvolumeModelProperties.CreationTimeStamp.ToTime()
	}

	return original, nil
}

func (c Client) subvolumesClientListByVolume(ctx context.Context, resourceGroup, netappAccount,
	capacityPool, volumeName string,
) (*netapp.SubvolumesListPage, error) {
	var subvolumesInfoList netapp.SubvolumesListPage
	var err error

	volumeFullName := CreateVolumeFullName(resourceGroup, netappAccount, capacityPool, volumeName)
	api := "SubvolumesClient.ListByVolume"
	logFields := log.Fields{
		"API":    api,
		"volume": volumeFullName,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		subvolumesInfoList, err = c.sdkClient.SubvolumesClient.ListByVolume(sdkCtx, resourceGroup, netappAccount,
			capacityPool, volumeName)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error listing subvolumes on volume.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Listing subvolumes on volume.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to list subvolumes by volume: %v", err)
		}
		return nil, err
	}

	logFields["correlationID"] = GetCorrelationID(subvolumesInfoList.Response().Response.Response)
	Logc(ctx).WithFields(logFields).Debug("Read subvolumes from volume.")

	return &subvolumesInfoList, nil
}

// SubvolumesForVolume returns a list of subvolume on a volume.
func (c Client) SubvolumesForVolume(ctx context.Context, filesystem *FileSystem) (*[]*Subvolume, error) {
	subvolumeInfoList, err := c.subvolumesClientListByVolume(ctx, filesystem.ResourceGroup,
		filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
	if err != nil {
		return nil, err
	}

	var subvolumes []*Subvolume

	for subvolumeInfoList.NotDone() {
		subvolumeInfoList, err = c.subvolumesClientNextWithContext(ctx, subvolumeInfoList)
		if err != nil {
			return nil, fmt.Errorf("error iterating subvolumeInfo list: %v", err)
		}

		subvolumeInfos := subvolumeInfoList.Values()

		for idx := range subvolumeInfos {

			subvolume, subvolumeErr := c.newSubvolumeFromSubvolumeInfo(ctx, &subvolumeInfos[idx])
			if subvolumeErr != nil {
				Logc(ctx).WithError(subvolumeErr).Errorf("Internal error creating internal Subvolume object.")
				return nil, subvolumeErr
			}

			subvolumes = append(subvolumes, subvolume)
		}
	}

	return &subvolumes, nil
}

// subvolumesClientNextWithContext advances to the next page of the subvolumes list page
func (c Client) subvolumesClientNextWithContext(ctx context.Context, subvolumeInfoList *netapp.SubvolumesListPage) (
	*netapp.SubvolumesListPage, error,
) {
	var err error

	api := "SubvolumesListPage.NextWithContext"
	logFields := log.Fields{
		"API": api,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		err = subvolumeInfoList.NextWithContext(sdkCtx)
		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error iterating subvolumeInfo list.")
				return backoff.Permanent(err)
			}
		}
		return nil
	}

	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Iterating subvolumeInfo list.")

	if err = backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to iterate subvolumeInfo list: %v", err)
		}
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Iterated subvolumeInfo list.")

	return subvolumeInfoList, err
}

// Subvolumes returns a list of all subvolumes.
func (c Client) Subvolumes(ctx context.Context, fileVolumePools []string) (*[]*Subvolume, error) {
	var subvolumes []*Subvolume

	for _, fileVolume := range fileVolumePools {
		resourceGroup, netappAccount, cpoolName, volumeName, err := ParseVolumeName(fileVolume)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Error getting volumes path details from %s.", fileVolume)
			return nil, err
		}

		fs := &FileSystem{
			ResourceGroup: resourceGroup,
			NetAppAccount: netappAccount,
			CapacityPool:  cpoolName,
			Name:          volumeName,
		}

		subvolumesList, err := c.SubvolumesForVolume(ctx, fs)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Error fetching subvolumes from pool %s.", fileVolume)
			return nil, err
		}

		subvolumes = append(subvolumes, *subvolumesList...)
	}

	return &subvolumes, nil
}

// Subvolume uses a volume config record to fetch a subvolume by the most efficient means.
func (c Client) Subvolume(
	ctx context.Context, volConfig *storage.VolumeConfig, queryMetadata bool,
) (*Subvolume, error) {
	// Subvolume ID must be present
	if volConfig.InternalID != "" {
		return c.SubvolumeByID(ctx, volConfig.InternalID, queryMetadata)
	}

	return nil, fmt.Errorf("volume config internal ID not found")
}

// SubvolumeParentVolume uses a volume config record to fetch a subvolume's parent volume.
func (c Client) SubvolumeParentVolume(ctx context.Context, volConfig *storage.VolumeConfig) (*FileSystem, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		subscriptionID, resourceGroup, _, netappAccount, cPoolName, volumeName, _,
			err := ParseSubvolumeID(volConfig.InternalID)
		if err != nil {
			return nil, err
		}

		volume, err := c.VolumeByID(ctx, CreateVolumeID(subscriptionID, resourceGroup, netappAccount, cPoolName,
			volumeName))
		if err != nil {
			return nil, err
		}

		return volume, nil
	}

	return nil, fmt.Errorf("volume config internal ID not found")
}

// SubvolumeExists uses a volume config record to look for a Subvolume by the most efficient means.
func (c Client) SubvolumeExists(
	ctx context.Context, volConfig *storage.VolumeConfig, candidateFileVolumePools []string,
) (bool, *Subvolume, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return c.SubvolumeExistsByID(ctx, volConfig.InternalID)
	}

	// Fall back to the creation token, here expectation is that it is called in the event of Create, Clone or their
	// failures when the name of the subvolume is deterministic.
	return c.SubvolumeExistsByCreationToken(ctx, volConfig.InternalName, candidateFileVolumePools)
}

// SubvolumeExistsByCreationToken checks whether a subvolume exists using its creation token as a key.
func (c Client) SubvolumeExistsByCreationToken(
	ctx context.Context, creationToken string, candidateFileVolumePools []string,
) (bool, *Subvolume, error) {
	if subvolume, err := c.SubvolumeByCreationToken(ctx, creationToken, candidateFileVolumePools, false); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	} else {
		return true, subvolume, nil
	}
}

// SubvolumeByCreationToken fetches a Subvolume by its creation token.  We can't query the SDK for
// subvolume by creation token, so our only choice here is to check for subvolume presence in each of the volume.
// That is obviously very inefficient, so this method should be used only when absolutely necessary.
func (c Client) SubvolumeByCreationToken(
	ctx context.Context, creationToken string, candidateFileVolumePools []string, queryMetadata bool,
) (*Subvolume, error) {
	Logc(ctx).Debugf("Fetching subvolume by creation token %s.", creationToken)

	matchingSubvolumes := make([]*Subvolume, 0)

	for _, filePoolVolume := range candidateFileVolumePools {
		resourceGroup, netappAccount, cpoolName, volumeName, err := ParseVolumeName(filePoolVolume)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Error getting file pool volume path details from %s.", filePoolVolume)
			return nil, err
		}

		subvolumeID := CreateSubvolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, cpoolName, volumeName,
			creationToken)

		subvolume, err := c.SubvolumeByID(ctx, subvolumeID, false)
		if err != nil {
			if utils.IsNotFoundError(err) {
				continue
			}

			Logc(ctx).WithError(err).Errorf("Error checking for subvolume '%s' in pool '%s'.", subvolumeID,
				filePoolVolume)
			return nil, err
		}

		matchingSubvolumes = append(matchingSubvolumes, subvolume)
	}

	switch len(matchingSubvolumes) {
	case 0:
		return nil, utils.NotFoundError(fmt.Sprintf("subvolume with creation token '%s' not found", creationToken))
	case 1:
		// This subvolume object does not contain metadata as it requires talking to actual storage
		// and the delay exceeds Azure 1 second time limit.
		if queryMetadata {
			Logc(ctx).Debugf("Fetching subvolume %s metadata.", matchingSubvolumes[0].Name)
			return c.SubvolumeMetadata(ctx, matchingSubvolumes[0])
		} else {
			return matchingSubvolumes[0], nil
		}

	default:
		return nil, utils.NotFoundError(fmt.Sprintf("multiple subvolumes with creation token '%s' found",
			creationToken))
	}
}

func (c Client) subvolumesClientGet(ctx context.Context, subvolumeID string) (*netapp.SubvolumeInfo, error) {
	var subvolumeInfo netapp.SubvolumeInfo
	var err error

	_, resourceGroup, _, netappAccount, capacityPool, volumeName, subvolumeName, err := ParseSubvolumeID(subvolumeID)
	if err != nil {
		return nil, err
	}

	api := "SubvolumesClient.Get"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolumeID,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		subvolumeInfo, err = c.sdkClient.SubvolumesClient.Get(sdkCtx, resourceGroup, netappAccount, capacityPool, volumeName,
			subvolumeName)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else if IsANFNotFoundError(err) {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Debug("Subvolume not found by ID.")
				return backoff.Permanent(utils.NotFoundError(fmt.Sprintf("subvolume not found")))
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching subvolume by ID.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Getting subvolume by ID.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else if utils.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Warnf("Failed to get subvolume: %v", err)
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to get subvolume: %v", err)
		}
		return nil, err
	}

	logFields["correlationID"] = GetCorrelationID(subvolumeInfo.Response.Response)
	Logc(ctx).WithFields(logFields).Debug("Subvolume found by ID")

	return &subvolumeInfo, nil
}

// SubvolumeByID returns a Subvolume based on its Azure-style ID.
func (c Client) SubvolumeByID(ctx context.Context, subvolumeID string, queryMetadata bool) (*Subvolume, error) {
	Logc(ctx).Tracef("Fetching subvolume by ID %s.", subvolumeID)

	subvolumeInfo, err := c.subvolumesClientGet(ctx, subvolumeID)
	if err != nil {
		return nil, err
	}

	subvolume, err := c.newSubvolumeFromSubvolumeInfo(ctx, subvolumeInfo)
	if err != nil {
		return nil, err
	}

	// GET does not fetch metadata as it requires talking to actual storage and the delay exceeds Azure 1 second time
	// limit
	if queryMetadata {
		Logc(ctx).Debugf("Fetching subvolume %s metadata.", subvolumeID)
		return c.SubvolumeMetadata(ctx, subvolume)
	} else {
		return subvolume, nil
	}
}

func (c Client) subvolumesClientGetMetadata(ctx context.Context,
	subvolume *Subvolume,
) (*netapp.SubvolumesGetMetadataFuture, error) {
	var future netapp.SubvolumesGetMetadataFuture
	var err error

	api := "SubvolumesClient.GetMetadata"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolume.ID,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		future, err = c.sdkClient.SubvolumesClient.GetMetadata(sdkCtx, subvolume.ResourceGroup,
			subvolume.NetAppAccount, subvolume.CapacityPool, subvolume.Volume, subvolume.Name)
		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else if IsANFNotFoundError(err) {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Debug("Subvolume not found.")
				return backoff.Permanent(utils.NotFoundError(fmt.Sprintf("subvolume not found")))
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching subvolume metadata.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Get subvolume metadata.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to get subvolume metadata: %v", err)
		}
		return nil, err
	}

	logFields["correlationID"] = GetCorrelationID(future.Response())
	Logc(ctx).WithFields(logFields).Debug("Async request to retrieve metadata sent.")

	return &future, nil
}

func (c Client) subvolumesClientWaitForMetadata(ctx context.Context,
	subvolumeID string, future *netapp.SubvolumesGetMetadataFuture,
) error {
	var err error

	api := "SubvolumesGetMetadataFuture.WaitForCompletionRef"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolumeID,
	}

	invoke := func() error {
		err = future.WaitForCompletionRef(ctx, c.sdkClient.SubvolumesClient.Client)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Debug("Unable to retrieve subvolume metadata.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Waiting to get subvolume metadata.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failure when waiting to retrieve subvolume metadata: %v", err)
		}
		return err
	}

	logFields["correlationID"] = ""
	Logc(ctx).WithFields(logFields).Debug("Subvolume metadata found by ID")

	return nil
}

// SubvolumeMetadata returns a Subvolume metadata
func (c Client) SubvolumeMetadata(ctx context.Context, subvolume *Subvolume) (*Subvolume, error) {
	Logc(ctx).Tracef("Fetching subvolume metadata %s.", subvolume.FullName)

	logFields := log.Fields{
		"subvolumeID": subvolume.ID,
	}

	subvolumesGetMetadataFuture, err := c.subvolumesClientGetMetadata(ctx, subvolume)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Waiting for the request to complete.")

	if err = c.subvolumesClientWaitForMetadata(ctx, subvolume.ID, subvolumesGetMetadataFuture); err != nil {
		return nil, err
	}

	subvolumeModel, err := c.subvolumesClientGetMetadataFutureResult(ctx, subvolume, subvolumesGetMetadataFuture)
	if err != nil {
		return nil, err
	}

	return c.updateSubvolumeFromSubvolumeModel(ctx, subvolume, &subvolumeModel)
}

// subvolumesClientGetMetadataFutureResult returns the result of the subvolumeMetadataFuture call
func (c Client) subvolumesClientGetMetadataFutureResult(ctx context.Context, subvolume *Subvolume,
	subvolumesGetMetadataFuture *netapp.SubvolumesGetMetadataFuture,
) (netapp.SubvolumeModel, error) {
	var subvolumeModel netapp.SubvolumeModel
	var err error
	api := "SubvolumesGetMetadataFuture.Result"

	logFields := log.Fields{
		"subvolumeID": subvolume.ID,
		"API":         api,
	}

	invoke := func() error {
		subvolumeModel, err = subvolumesGetMetadataFuture.Result(c.sdkClient.SubvolumesClient)
		if err != nil {

			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				logFields["subvolume.FullName"] = subvolume.FullName
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to retrieve subvolume get metadata future result.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}

	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Retrieving subvolume get metadata result.")

	if err = backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to retrieve subvolume get metadata result: %v", err)
		}

		return netapp.SubvolumeModel{}, fmt.Errorf("unable to retrieve subvolume get metadata result for %v: %v",
			subvolume.FullName, err)
	}

	logFields["correlationID"] = GetCorrelationID(subvolumeModel.Response.Response)
	Logc(ctx).WithFields(logFields).Debug("Retrieved subvolume get metadata result.")

	return subvolumeModel, nil
}

// SubvolumeExistsByID checks whether a subvolume exists using its creation token as a key.
func (c Client) SubvolumeExistsByID(ctx context.Context, id string) (bool, *Subvolume, error) {
	if subvolume, err := c.SubvolumeByID(ctx, id, false); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	} else {
		return true, subvolume, nil
	}
}

// WaitForSubvolumeState watches for a desired subvolume state and returns when that state is achieved
func (c Client) WaitForSubvolumeState(
	ctx context.Context, subvolume *Subvolume, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) (string, error) {
	subvolumeState := ""

	logFields := log.Fields{
		"subvolumeID":  subvolume.ID,
		"desiredState": desiredState,
	}

	checkSubvolumeState := func() error {
		var err error

		subvol, err := c.SubvolumeByID(ctx, subvolume.ID, false)
		if err != nil {
			subvolumeState = ""
			// This is a bit of a hack, but there is no 'Deleted' state in Azure -- the
			// volume just vanishes.  If we failed to query the volume info and we're trying
			// to transition to StateDeleted, try a raw fetch of the volume and if we
			// get back a 404, then call it a day.  Otherwise, log the error as usual.
			if desiredState == StateDeleted {
				if utils.IsNotFoundError(err) {
					// Deleted!
					Logc(ctx).Debugf("Implied deletion for volume %s", subvolume.Name)
					subvolumeState = StateDeleted
					return nil
				} else {
					return fmt.Errorf("waitForVolumeState internal error re-checking subvolume for deletion: %v", err)
				}
			}

			return fmt.Errorf("could not get subvolume status; %v", err)
		}

		subvolumeState = subvol.ProvisioningState

		if subvolumeState == desiredState {
			return nil
		}

		err = fmt.Errorf("subvolume state is %s, not %s", subvolumeState, desiredState)

		// Return a permanent error to stop retrying if we reached one of the abort states
		if utils.SliceContainsString(abortStates, subvolumeState) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}
	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for subvolume state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 3 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Info("Waiting for subvolume state.")

	if err := backoff.RetryNotify(checkSubvolumeState, stateBackoff, stateNotify); err != nil {
		if IsTerminalStateError(err) {
			Logc(ctx).WithFields(logFields).Errorf("Subvolume reached terminal state.")
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Subvolume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return subvolumeState, err
	}

	Logc(ctx).WithFields(logFields).Debug("Desired subvolume state reached.")

	return subvolumeState, nil
}

func (c Client) subvolumesClientCreate(ctx context.Context, newSubvol netapp.SubvolumeInfo,
	resourceGroup, netappAccount, capacityPool, volumeName, subvolumeName string,
) error {
	var future netapp.SubvolumesCreateFuture
	var err error

	subvolumeID := CreateSubvolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, capacityPool, volumeName,
		subvolumeName)

	api := "SubvolumesClient.Create"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolumeID,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		future, err = c.sdkClient.SubvolumesClient.Create(sdkCtx, newSubvol, resourceGroup, netappAccount,
			capacityPool, volumeName, subvolumeName)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating subvolume.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Creating subvolume.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to create subvolume: %v", err)
		}
		return err
	}

	logFields["correlationID"] = GetCorrelationID(future.Response())
	Logc(ctx).WithFields(logFields).Debug("Subvolume create request sent.")

	return nil
}

// CreateSubvolume creates a new subvolume
func (c Client) CreateSubvolume(ctx context.Context, request *SubvolumeCreateRequest) (*Subvolume, error) {
	var err error

	resourceGroup, netappAccount, cpoolName, volumeName, err := ParseVolumeName(request.Volume)
	if err != nil {
		return nil, fmt.Errorf("unable to get volume information: %v", err)
	}

	subvolumeFullName := CreateSubvolumeFullName(resourceGroup, netappAccount, cpoolName, volumeName, request.CreationToken)

	path := "/" + request.CreationToken
	newSubvol := netapp.SubvolumeInfo{
		Name: &request.CreationToken,
		SubvolumeProperties: &netapp.SubvolumeProperties{
			Path: &path,
		},
	}

	if request.Size > 0 {
		newSubvol.SubvolumeProperties.Size = &request.Size
	}

	if request.Parent != "" {
		parentPath := "/" + request.Parent
		newSubvol.SubvolumeProperties.ParentPath = &parentPath
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.CreationToken,
		"resourceGroup": resourceGroup,
		"netAppAccount": netappAccount,
		"capacityPool":  cpoolName,
		"volumeName":    volumeName,
	}).Debug("Issuing subvolume create request.")

	err = c.subvolumesClientCreate(ctx, newSubvol, resourceGroup, netappAccount, cpoolName, volumeName,
		request.CreationToken)
	if err != nil {
		return nil, err
	}

	// The subvolume doesn't exist yet, so forge the subvolume ID to enable conversion to a Subvolume struct
	newSubvolumeID := CreateSubvolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, cpoolName, volumeName,
		request.CreationToken)
	newSubvol.ID = &newSubvolumeID

	subvol, err := c.newSubvolumeFromSubvolumeInfo(ctx, &newSubvol)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"name": subvolumeFullName,
	}).Info("Subvolume created.")

	return subvol, nil
}

func (c Client) subvolumesClientUpdate(
	ctx context.Context, patch *netapp.SubvolumePatchRequest, subvolume *Subvolume,
) error {
	var future netapp.SubvolumesUpdateFuture
	var err error

	api := "SubvolumesClient.Update"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolume.ID,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		future, err = c.sdkClient.SubvolumesClient.Update(
			sdkCtx, *patch, subvolume.ResourceGroup, subvolume.NetAppAccount, subvolume.CapacityPool,
			subvolume.Volume, subvolume.Name)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error resizing subvolume.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Resizing subvolume.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to resize subvolume: %v", err)
		}
		return err
	}

	logFields["correlationID"] = GetCorrelationID(future.Response())
	Logc(ctx).WithFields(logFields).Debug("Subvolume resized.")

	return nil
}

// ResizeSubvolume sends a SubvolumePatchRequest to update a subvolume's size.
func (c Client) ResizeSubvolume(ctx context.Context, subvolume *Subvolume, newSizeBytes int64) error {
	path := "/" + subvolume.Name
	patch := &netapp.SubvolumePatchRequest{
		SubvolumePatchParams: &netapp.SubvolumePatchParams{
			Size: &newSizeBytes,
			Path: &path,
		},
	}

	err := c.subvolumesClientUpdate(ctx, patch, subvolume)
	if err != nil {
		return err
	}

	return nil
}

func (c Client) subvolumesClientDelete(ctx context.Context, subvolume *Subvolume) error {
	var future netapp.SubvolumesDeleteFuture
	var err error

	api := "SubvolumesClient.Delete"
	logFields := log.Fields{
		"API":         api,
		"subvolumeID": subvolume.ID,
	}

	invoke := func() error {
		sdkCtx, cancel := context.WithTimeout(ctx, DefaultSubvolumeSDKTimeout)
		defer cancel()

		future, err = c.sdkClient.SubvolumesClient.Delete(sdkCtx, subvolume.ResourceGroup,
			subvolume.NetAppAccount, subvolume.CapacityPool, subvolume.Volume, subvolume.Name)

		if err != nil {
			logFields["correlationID"] = GetCorrelationIDFromError(err)

			if IsANFTooManyRequestsError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Error(
					"Unable to receive response due to API throttling.")
				return utils.TooManyRequestsError("unable to receive response due to API throttling")
			} else if IsANFNotFoundError(err) {
				Logc(ctx).WithFields(logFields).WithError(err).Info("Subvolume already deleted")
				return backoff.Permanent(utils.NotFoundError(fmt.Sprintf("subvolume %s not found", subvolume.Name)))
			} else {
				// Return a permanent error for non HTTP 429 case
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting subvolume.")
				return backoff.Permanent(err)
			}
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
			"API":       api,
		}).Debugf("Retrying API.")
	}
	invokeBackoff := c.SubvolumeCreateBackOff()

	Logc(ctx).WithFields(logFields).Info("Deleting subvolume.")

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		if utils.IsTooManyRequestsError(err) {
			Logc(ctx).WithFields(logFields).Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		} else if utils.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Warnf("Failed to delete subvolume: %v", err)
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Failed to delete subvolume: %v", err)
		}

		return err
	}

	logFields["correlationID"] = GetCorrelationID(future.Response())
	Logc(ctx).WithFields(logFields).Debug("Subvolume deleted.")

	return nil
}

// DeleteSubvolume deletes a subvolume.
func (c Client) DeleteSubvolume(ctx context.Context, subvolume *Subvolume) error {
	if err := c.subvolumesClientDelete(ctx, subvolume); err != nil {
		return err
	}

	return nil
}

// ValidateFilePoolVolumes validates a filePoolVolume and its location
func (c Client) ValidateFilePoolVolumes(
	ctx context.Context, filePoolVolumeNames []string,
) ([]*FileSystem, error) {
	// Ensure the length of filePoolVolumeNames is 1
	if len(filePoolVolumeNames) != 1 {
		return nil, fmt.Errorf("the config should contain exactly one entry in filePoolVolumes, "+
			"it has %d entries", len(filePoolVolumeNames))
	}

	var volumes []*FileSystem

	for _, filePoolVolumeName := range filePoolVolumeNames {
		resourceGroup, netappAccount, cpoolName, volumeName, err := ParseVolumeName(filePoolVolumeName)
		if err != nil {
			return nil, err
		}

		volume, err := c.VolumeByID(ctx, CreateVolumeID(c.config.SubscriptionID, resourceGroup, netappAccount,
			cpoolName, volumeName))
		if err != nil {
			return nil, err
		}

		if volume.Location != c.config.Location {
			return nil, fmt.Errorf("filePoolVolumes validation failed; location of the volume filePoolVolumeNames is"+
				" not %s but %s", c.config.Location,
				volume.Location)
		}

		if volume.SubvolumesEnabled == false {
			return nil, fmt.Errorf("filePoolVolumes validation failed; volume '%s' does not support subvolumes",
				filePoolVolumeName)
		}

		volumes = append(volumes, volume)
	}

	return volumes, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Miscellaneous utility functions and error types
// ///////////////////////////////////////////////////////////////////////////////

// IsANFNotFoundError checks whether an error returned from the ANF SDK contains a 404 (Not Found) error.
func IsANFNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if detailedErr, ok := err.(autorest.DetailedError); ok {
		if detailedErr.Response != nil && detailedErr.Response.StatusCode == http.StatusNotFound {
			return true
		}
	}

	return false
}

// IsANFTooManyRequestsError checks whether an error returned from the ANF SDK contains a 429 (Too Many Requests) error.
func IsANFTooManyRequestsError(err error) bool {
	if err == nil {
		return false
	}

	if detailedErr, ok := err.(autorest.DetailedError); ok {
		if detailedErr.Response != nil && detailedErr.Response.StatusCode == http.StatusTooManyRequests {
			return true
		}
	}

	return false
}

// GetCorrelationIDFromError accepts an error returned from the ANF SDK and extracts the correlation
// header, if present.
func GetCorrelationIDFromError(err error) (id string) {
	if err == nil {
		return
	}

	if detailedErr, ok := err.(autorest.DetailedError); ok {
		id = GetCorrelationID(detailedErr.Response)
	}

	return
}

// GetCorrelationID accepts an HTTP response returned from the ANF SDK and extracts the correlation
// header, if present.
func GetCorrelationID(response *http.Response) (id string) {
	if response == nil || response.Header == nil {
		return
	}

	if ids, ok := response.Header[CorrelationIDHeader]; ok && len(ids) > 0 {
		id = ids[0]
	}

	return
}

func (c Client) SubvolumeCreateBackOff() *backoff.ExponentialBackOff {
	invokeBackoff := backoff.NewExponentialBackOff()
	invokeBackoff.MaxElapsedTime = c.config.SDKTimeout
	invokeBackoff.RandomizationFactor = 0.1
	invokeBackoff.InitialInterval = 1 * time.Second
	invokeBackoff.Multiplier = 1.414

	return invokeBackoff
}

// DerefString accepts a string pointer and returns the value of the string, or "" if the pointer is nil.
func DerefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// DerefStringArray accepts a string array pointer and returns the value of the array, or []string if the pointer is nil.
func DerefStringArray(s *[]string) []string {
	if s != nil {
		return *s
	}
	return make([]string, 0)
}

// DerefBool accepts a bool pointer and returns the value of the bool, or false if the pointer is nil.
func DerefBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

// DerefInt32 accepts an int32 pointer and returns the value of the int32, or 0 if the pointer is nil.
func DerefInt32(i *int32) int32 {
	if i != nil {
		return *i
	}
	return 0
}

// DerefInt64 accepts an int64 pointer and returns the value of the int64, or 0 if the pointer is nil.
func DerefInt64(i *int64) int64 {
	if i != nil {
		return *i
	}
	return 0
}

// TerminalStateError signals that the object is in a terminal state.  This is used to stop waiting on
// an object to change state.
type TerminalStateError struct {
	Err error
}

func (e *TerminalStateError) Error() string {
	return e.Err.Error()
}

// TerminalState wraps the given err in a *TerminalStateError.
func TerminalState(err error) *TerminalStateError {
	return &TerminalStateError{
		Err: err,
	}
}

func IsTerminalStateError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*TerminalStateError)
	return ok
}
