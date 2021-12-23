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

	"github.com/Azure/azure-sdk-for-go/services/netapp/mgmt/2021-06-01/netapp"
	"github.com/Azure/azure-sdk-for-go/services/resourcegraph/mgmt/2021-03-01/resourcegraph"
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
	VolumeCreateTimeout = 10 * time.Second
	SnapshotTimeout     = 240 * time.Second // Snapshotter sidecar has a timeout of 5 minutes.  Stay under that!
	DefaultTimeout      = 120 * time.Second
	MaxLabelLength      = 256
	DefaultSDKTimeout   = 30 * time.Second
	CorrelationIDHeader = "X-Ms-Correlation-Request-Id"
)

var (
	capacityPoolIDRegex = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)$`)
	volumeIDRegex       = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)$`)
	snapshotIDRegex     = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)/snapshots/(?P<snapshot>[^/]+)$`)
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
	AuthConfig      azauth.ClientCredentialsConfig
	GraphClient     resourcegraph.BaseClient
	VolumesClient   netapp.VolumesClient
	SnapshotsClient netapp.SnapshotsClient
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
		AuthConfig:      azauth.NewClientCredentialsConfig(config.ClientID, config.ClientSecret, config.TenantID),
		GraphClient:     resourcegraph.New(),
		VolumesClient:   netapp.NewVolumesClient(config.SubscriptionID),
		SnapshotsClient: netapp.NewSnapshotsClient(config.SubscriptionID),
	}

	// Set authorization endpoints
	sdkClient.AuthConfig.AADEndpoint = azure.PublicCloud.ActiveDirectoryEndpoint
	sdkClient.AuthConfig.Resource = azure.PublicCloud.ResourceManagerEndpoint

	// Plumb the authorization through to sub-clients.
	if sdkClient.GraphClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.VolumesClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
		return nil, err
	}
	if sdkClient.SnapshotsClient.Authorizer, err = sdkClient.AuthConfig.Authorizer(); err != nil {
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
func CreateVolumeID(subscriptionID, resourceGroup, netappAccount string, capacityPool string, volume string) string {
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

// CreateSnapshotID creates the Azure-style ID for a snapshot.
func CreateSnapshotID(
	subscriptionID, resourceGroup, netappAccount string, capacityPool string, volume, snapshot string,
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

// ///////////////////////////////////////////////////////////////////////////////
// Functions to convert between ANF SDK & internal volume structs
// ///////////////////////////////////////////////////////////////////////////////

// exportPolicyExport turns an internal ExportPolicy into something consumable by the SDK.
func exportPolicyExport(exportPolicy *ExportPolicy) *netapp.VolumePropertiesExportPolicy {

	var anfRules = make([]netapp.ExportPolicyRule, 0)

	for _, rule := range exportPolicy.Rules {

		anfRule := netapp.ExportPolicyRule{
			RuleIndex:      &rule.RuleIndex,
			UnixReadOnly:   &rule.UnixReadOnly,
			UnixReadWrite:  &rule.UnixReadWrite,
			Cifs:           &rule.Cifs,
			Nfsv3:          &rule.Nfsv3,
			Nfsv41:         &rule.Nfsv41,
			AllowedClients: &rule.AllowedClients,
		}

		anfRules = append(anfRules, anfRule)
	}

	return &netapp.VolumePropertiesExportPolicy{
		Rules: &anfRules,
	}
}

// exportPolicyImport turns an SDK ExportPolicy into an internal one.
func exportPolicyImport(anfExportPolicy *netapp.VolumePropertiesExportPolicy) *ExportPolicy {

	var rules = make([]ExportRule, 0)

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
	}, nil
}

// getMountTargetsFromVolume extracts the mount targets from an SDK volume.
func (c Client) getMountTargetsFromVolume(ctx context.Context, vol *netapp.Volume) []MountTarget {

	mounts := make([]MountTarget, 0)

	if vol.MountTargets == nil {
		Logc(ctx).Tracef("Volume %s has nil MountTargetProperties.", *vol.Name)
		return mounts
	}

	for _, mtp := range *vol.MountTargets {

		var mt = MountTarget{
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

		for _, v := range volumes {
			filesystem, fsErr := c.newFileSystemFromVolume(ctx, &v)
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
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name)
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
		return nil, fmt.Errorf("snapshot %s has no location", *anfSnapshot.Location)
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

	for _, anfSnapshot := range *snapshotList.Value {

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

	var anfSnapshot = netapp.Snapshot{
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
