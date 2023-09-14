// Copyright 2022 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Azure NetApp Files SDK
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	netapp "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/netapp/armnetapp/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	resourcegraph "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	features "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armfeatures"
	"github.com/cenkalti/backoff/v4"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/nodeipam/ipam/cidrset"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const (
	VolumeCreateTimeout        = 10 * time.Second
	SnapshotTimeout            = 240 * time.Second // Snapshotter sidecar has a timeout of 5 minutes.  Stay under that!
	DefaultTimeout             = 120 * time.Second
	MaxLabelLength             = 256
	DefaultSDKTimeout          = 30 * time.Second
	DefaultSubvolumeSDKTimeout = 15 * time.Second
	SDKRetryDelay              = 2 * time.Second
	SDKMaxRetryDelay           = 15 * time.Second
	CorrelationIDHeader        = "X-Ms-Correlation-Request-Id"
	SubvolumeNameSeparator     = "-file-"
	DefaultCredentialFilePath  = "/etc/kubernetes/azure.json"
	DefaultAccountNamePrefix   = "aks-anf-account-"
	DefaultPoolNamePrefix      = "aks-anf-pool-"
	DefaultSubnetNamePrefix    = "aks-anf-subnet-"
	DefaultServiceLevel        = netapp.ServiceLevelStandard
	DefaultPoolSize            = 10 * 1099511627776 // 10 TiB
	DefaultSubnetMaskSize      = 24
)

var (
	capacityPoolIDRegex = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)$`)
	volumeIDRegex       = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)$`)
	volumeNameRegex     = regexp.MustCompile(`/?(?P<resourceGroup>[^/]+)/(?P<netappAccount>[^/]+)/(?P<capacityPool>[^/]+)/(?P<volume>[^/]+)?/?$`)
	snapshotIDRegex     = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)/snapshots/(?P<snapshot>[^/]+)$`)
	subvolumeIDRegex    = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/netAppAccounts/(?P<netappAccount>[^/]+)/capacityPools/(?P<capacityPool>[^/]+)/volumes/(?P<volume>[^/]+)/subvolumes/(?P<subvolume>[^/]+)$`)
	subnetIDRegex       = regexp.MustCompile(`^/subscriptions/(?P<subscriptionID>[^/]+)/resourceGroups/(?P<resourceGroup>[^/]+)/providers/(?P<provider>[^/]+)/virtualNetworks/(?P<virtualNetwork>[^/]+)/subnets/(?P<subnet>[^/]+)$`)
)

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	// Azure API authentication parameters
	azclient.AzureAuthConfig
	SubscriptionID    string `json:"subscriptionId"`
	Location          string `json:"location"`
	ResourceGroup     string `json:"resourceGroup"`
	VnetName          string `json:"vnetName"`
	VnetResourceGroup string `json:"vnetResourceGroup"`
	StorageDriverName string

	// Options
	DebugTraceFlags map[string]bool
	SDKTimeout      time.Duration // Timeout applied to all calls to the Azure SDK
	MaxCacheAge     time.Duration // The oldest data we should expect in the cached resources
}

// AzureClient holds operational Azure SDK objects.
type AzureClient struct {
	Credential       azcore.TokenCredential
	FeaturesClient   *features.Client
	GraphClient      *resourcegraph.Client
	VolumesClient    *netapp.VolumesClient
	SnapshotsClient  *netapp.SnapshotsClient
	SubvolumesClient *netapp.SubvolumesClient
	AccountClient    *netapp.AccountsClient
	PoolsClient      *netapp.PoolsClient
	SubnetsClient    *armnetwork.SubnetsClient
	VnetClient       *armnetwork.VirtualNetworksClient
	AzureResources
}

type PollerSVCreateResponse struct {
	*runtime.Poller[netapp.SubvolumesClientCreateResponse]
}
type PollerSVDeleteResponse struct {
	*runtime.Poller[netapp.SubvolumesClientDeleteResponse]
}

type PollerResponse interface {
	Result(context.Context) error
}

func (p *PollerSVCreateResponse) Result(ctx context.Context) error {
	if p != nil && p.Poller != nil {
		var rawResponse *http.Response
		responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

		_, err := p.PollUntilDone(responseCtx, &runtime.PollUntilDoneOptions{Frequency: 2 * time.Second})
		if err != nil {
			Logc(ctx).WithError(err).Error("Error polling for subvolume create result.")
			return GetMessageFromError(ctx, err)
		}

		Logc(ctx).Debug("Result received for subvolume create.")
	}

	return nil
}

func (p *PollerSVDeleteResponse) Result(ctx context.Context) error {
	if p != nil && p.Poller != nil {
		var rawResponse *http.Response
		responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

		_, err := p.PollUntilDone(responseCtx, &runtime.PollUntilDoneOptions{Frequency: 2 * time.Second})
		if err != nil {
			Logc(ctx).WithError(err).Error("Error polling for subvolume delete result.")
			return GetMessageFromError(ctx, err)
		}

		Logc(ctx).Debug("Result received for subvolume delete.")
	}

	return nil
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
	credential, err := GetAzureCredential(config)
	if err != nil {
		return nil, err
	}

	clientOptions := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Retry: policy.RetryOptions{
				TryTimeout:    config.SDKTimeout,
				RetryDelay:    SDKRetryDelay,
				MaxRetryDelay: SDKMaxRetryDelay,
			},
		},
	}

	subvolumeClientOptions := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Retry: policy.RetryOptions{
				MaxRetries:    6, // 30 seconds, assuming hardcoded Retry-After value of 5 seconds
				TryTimeout:    DefaultSubvolumeSDKTimeout,
				RetryDelay:    SDKRetryDelay,
				MaxRetryDelay: SDKMaxRetryDelay,
			},
		},
	}

	featuresClient, err := features.NewClient(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	graphClient, err := resourcegraph.NewClient(credential, clientOptions)
	if err != nil {
		return nil, err
	}
	volumesClient, err := netapp.NewVolumesClient(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	snapshotsClient, err := netapp.NewSnapshotsClient(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	subvolumesClient, err := netapp.NewSubvolumesClient(config.SubscriptionID, credential, subvolumeClientOptions)
	if err != nil {
		return nil, err
	}
	accountClient, err := netapp.NewAccountsClient(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	poolsClient, err := netapp.NewPoolsClient(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	networkClientFactory, err := armnetwork.NewClientFactory(config.SubscriptionID, credential, clientOptions)
	if err != nil {
		return nil, err
	}
	subnetsClient := networkClientFactory.NewSubnetsClient()
	vnetClient := networkClientFactory.NewVirtualNetworksClient()

	sdkClient := &AzureClient{
		Credential:       credential,
		FeaturesClient:   featuresClient,
		GraphClient:      graphClient,
		VolumesClient:    volumesClient,
		SnapshotsClient:  snapshotsClient,
		SubvolumesClient: subvolumesClient,
		AccountClient:    accountClient,
		PoolsClient:      poolsClient,
		SubnetsClient:    subnetsClient,
		VnetClient:       vnetClient,
	}

	return Client{
		config:    &config,
		sdkClient: sdkClient,
	}, nil
}

func GetAzureCredential(config ClientConfig) (credential azcore.TokenCredential, err error) {
	clientOptions, err := azclient.GetDefaultAuthClientOption(nil)
	if err != nil {
		return nil, errors.New("error getting default auth client option: " + err.Error())
	}
	authProvider, err := azclient.NewAuthProvider(config.AzureAuthConfig, clientOptions)
	if err != nil {
		return nil, errors.New("error creating azure auth provider: " + err.Error())
	}
	return authProvider.GetAzIdentity()
}

// Init runs startup logic after allocating the driver resources.
func (c Client) Init(ctx context.Context, pools map[string]storage.Pool) error {
	// Map vpools to backend
	c.registerStoragePools(pools)

	// Find out what we have to work with in Azure
	err := c.RefreshAzureResources(ctx)
	if errors.IsNotFoundError(err) && shouldGenerateAzureResources(pools) {
		Logc(ctx).WithError(err).Info("Generate Azure resources when not found.")
		if err := c.GenerateAzureResources(ctx); err != nil {
			return err
		}
		message := "Azure resources generated successfully, reconcile again to update cache."
		Logc(ctx).Info(message)
		return errors.ReconcileDeferredError(message)
	}

	return err
}

func shouldGenerateAzureResources(pools map[string]storage.Pool) bool {
	for _, pool := range pools {
		if pool.InternalAttributes()[PVirtualNetwork] != "" ||
			pool.InternalAttributes()[PSubnet] != "" ||
			pool.InternalAttributes()[PResourceGroups] != "" ||
			pool.InternalAttributes()[PCapacityPools] != "" ||
			pool.InternalAttributes()[PNetappAccounts] != "" {
			return false
		}
	}
	return true
}

// GenerateAzureResources creates the Azure resources needed by Trident.
func (c Client) GenerateAzureResources(ctx context.Context) error {
	accountName, exist, err := c.accountExists(ctx)
	if err != nil {
		return err
	}
	if !exist {
		Logc(ctx).Info("Creating Azure NetApp Account")
		accountName = DefaultAccountNamePrefix + utils.RandomNumber(8)
		accountPollerResp, err := c.sdkClient.AccountClient.BeginCreateOrUpdate(
			ctx,
			c.config.ResourceGroup,
			accountName,
			netapp.Account{
				Location: to.Ptr(c.config.Location),
			}, nil)
		if err != nil {
			return err
		}
		_, err = accountPollerResp.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
	}

	exist, err = c.poolExists(ctx, accountName)
	if err != nil {
		return err
	}
	if !exist {
		Logc(ctx).Info("Creating Azure NetApp Capacity Pools")
		poolName := DefaultPoolNamePrefix + utils.RandomNumber(8)
		poolPollerResp, err := c.sdkClient.PoolsClient.BeginCreateOrUpdate(
			ctx,
			c.config.ResourceGroup,
			accountName,
			poolName,
			netapp.CapacityPool{
				Location: to.Ptr(c.config.Location),
				Properties: &netapp.PoolProperties{
					ServiceLevel: to.Ptr(DefaultServiceLevel),
					Size:         to.Ptr[int64](DefaultPoolSize),
				},
			}, nil)
		if err != nil {
			return err
		}
		_, err = poolPollerResp.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
	}

	vnetGetResp, exist, err := c.subnetExists(ctx)
	if err != nil {
		return err
	}
	if !exist {
		Logc(ctx).Info("Get Available Subnet address range")
		vnetAddressSpace := vnetGetResp.Properties.AddressSpace.AddressPrefixes[0]
		var existingSubnetAddressSpaces []*string
		for _, subnet := range vnetGetResp.Properties.Subnets {
			existingSubnetAddressSpaces = append(existingSubnetAddressSpaces, subnet.Properties.AddressPrefix)
			existingSubnetAddressSpaces = append(existingSubnetAddressSpaces, subnet.Properties.AddressPrefixes...)
		}
		availableSubnetAddressRange, err := getAvailableSubnetAddressRange(vnetAddressSpace, existingSubnetAddressSpaces, DefaultSubnetMaskSize)
		if err != nil {
			return err
		}

		Logc(ctx).Info("Creating and delegating subnet")
		subnetName := DefaultSubnetNamePrefix + utils.RandomNumber(8)
		subnetPollerResp, err := c.sdkClient.SubnetsClient.BeginCreateOrUpdate(
			ctx,
			c.config.VnetResourceGroup,
			c.config.VnetName,
			subnetName,
			armnetwork.Subnet{
				Properties: &armnetwork.SubnetPropertiesFormat{
					Delegations: to.SliceOfPtrs(armnetwork.Delegation{
						Name: to.Ptr("Microsoft.Netapp/volumes"),
						Properties: &armnetwork.ServiceDelegationPropertiesFormat{
							ServiceName: to.Ptr("Microsoft.Netapp/volumes"),
						},
					}),
					AddressPrefix: to.Ptr(availableSubnetAddressRange),
				},
			}, nil)
		if err != nil {
			return err
		}
		_, err = subnetPollerResp.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c Client) accountExists(ctx context.Context) (string, bool, error) {
	pager := c.sdkClient.AccountClient.NewListPager(c.config.ResourceGroup, nil)
	accounts := make([]*netapp.Account, 0)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return "", false, err
		}
		accounts = append(accounts, resp.Value...)
	}
	if len(accounts) == 0 {
		return "", false, nil
	}
	return *accounts[0].Name, true, nil
}

func (c Client) poolExists(ctx context.Context, accountName string) (bool, error) {
	pager := c.sdkClient.PoolsClient.NewListPager(c.config.ResourceGroup, accountName, nil)
	pools := make([]*netapp.CapacityPool, 0)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return false, err
		}
		pools = append(pools, resp.Value...)
	}
	return len(pools) > 0, nil
}

func (c Client) subnetExists(ctx context.Context) (*armnetwork.VirtualNetworksClientGetResponse, bool, error) {
	vnetGetResp, err := c.sdkClient.VnetClient.Get(ctx, c.config.VnetResourceGroup, c.config.VnetName, nil)
	if err != nil {
		return nil, false, err
	}
	for _, subnet := range vnetGetResp.Properties.Subnets {
		for _, delegation := range subnet.Properties.Delegations {
			if *delegation.Properties.ServiceName == "Microsoft.Netapp/volumes" {
				Logc(ctx).Infof("The 'Microsoft.NetApp/volumes' delegation can only exist on one subnet within a VNet. There is already a delegation on subnet %s, skip creating subnet", *subnet.Name)
				return nil, true, nil
			}
		}
	}
	return &vnetGetResp, false, nil
}

// getAvailableSubnetAddressRange returns an available subnet address range, which must be unique within the VNet address space
// and can't overlap with other subnet address ranges in the virtual network.
func getAvailableSubnetAddressRange(vnetAddressSpace *string, existingSubnetAddressSpaces []*string, maskSize int) (string, error) {
	_, vnetCIDR, _ := net.ParseCIDR(*vnetAddressSpace)
	cidrSet, err := cidrset.NewCIDRSet(vnetCIDR, maskSize)
	if err != nil {
		return "", err
	}

	var cidrIntersect = func(cidr1, cidr2 *net.IPNet) bool {
		return cidr1.Contains(cidr2.IP) || cidr2.Contains(cidr1.IP)
	}

	var cidr *net.IPNet
	for cidr, err = cidrSet.AllocateNext(); err == nil; cidr, err = cidrSet.AllocateNext() {
		for _, subnetAddressSpace := range existingSubnetAddressSpaces {
			_, subnetCIDR, _ := net.ParseCIDR(*subnetAddressSpace)
			if cidrIntersect(cidr, subnetCIDR) {
				goto CONTINUE
			}
		}
		return cidr.String(), nil
	CONTINUE:
	}

	return "", err
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
	anfRules := make([]*netapp.ExportPolicyRule, 0)

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

		anfRules = append(anfRules, &anfRule)
	}

	return &netapp.VolumePropertiesExportPolicy{
		Rules: anfRules,
	}
}

// exportPolicyImport turns an SDK ExportPolicy into an internal one.
func exportPolicyImport(anfExportPolicy *netapp.VolumePropertiesExportPolicy) *ExportPolicy {
	rules := make([]ExportRule, 0)

	if anfExportPolicy == nil || len(anfExportPolicy.Rules) == 0 {
		return &ExportPolicy{Rules: rules}
	}

	for _, anfRule := range anfExportPolicy.Rules {

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

	if vol.Properties == nil {
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
		ExportPolicy:      *exportPolicyImport(vol.Properties.ExportPolicy),
		Labels:            c.getLabelsFromVolume(vol),
		FileSystemID:      DerefString(vol.Properties.FileSystemID),
		ProvisioningState: DerefString(vol.Properties.ProvisioningState),
		CreationToken:     DerefString(vol.Properties.CreationToken),
		ProtocolTypes:     DerefStringPtrArray(vol.Properties.ProtocolTypes),
		QuotaInBytes:      DerefInt64(vol.Properties.UsageThreshold),
		ServiceLevel:      cPool.ServiceLevel,
		SnapshotDirectory: DerefBool(vol.Properties.SnapshotDirectoryVisible),
		SubnetID:          DerefString(vol.Properties.SubnetID),
		UnixPermissions:   DerefString(vol.Properties.UnixPermissions),
		MountTargets:      c.getMountTargetsFromVolume(ctx, vol),
		SubvolumesEnabled: c.getSubvolumesEnabledFromVolume(vol.Properties.EnableSubvolumes),
		NetworkFeatures:   DerefNetworkFeatures(vol.Properties.NetworkFeatures),
	}, nil
}

// getSubvolumesEnabledFromVolume extracts the SubvolumesEnabled from an SDK volume.
func (c Client) getSubvolumesEnabledFromVolume(value *netapp.EnableSubvolumes) bool {
	if value == nil || *value != netapp.EnableSubvolumesEnabled {
		return false
	}
	return true
}

// getMountTargetsFromVolume extracts the mount targets from an SDK volume.
func (c Client) getMountTargetsFromVolume(ctx context.Context, vol *netapp.Volume) []MountTarget {
	mounts := make([]MountTarget, 0)

	if vol.Properties.MountTargets == nil {
		Logc(ctx).Tracef("Volume %s has nil MountTargetProperties.", *vol.Name)
		return mounts
	}

	for _, mtp := range vol.Properties.MountTargets {

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
// Functions to retrieve and manage volumes
// ///////////////////////////////////////////////////////////////////////////////

// getVolumesFromPool gets a set of volumes belonging to a single capacity pool.  As pools can come and go
// in between cache updates, we ignore any 404 errors here.
func (c Client) getVolumesFromPool(ctx context.Context, cPool *CapacityPool) (*[]*FileSystem, error) {
	logFields := LogFields{
		"API":          "VolumesClient.NewListPager",
		"capacityPool": cPool.FullName,
	}

	var filesystems []*FileSystem

	pager := c.sdkClient.VolumesClient.NewListPager(cPool.ResourceGroup, cPool.NetAppAccount, cPool.Name, nil)

	for pager.More() {
		var rawResponse *http.Response
		responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

		nextResult, err := pager.NextPage(responseCtx)

		logFields["correlationID"] = GetCorrelationID(rawResponse)

		if err != nil {
			Logc(ctx).WithFields(logFields).Error("Could not iterate volumes.")
			return nil, fmt.Errorf("error iterating volumes; %v", err)
		}

		for _, volume := range nextResult.Value {
			filesystem, fsErr := c.newFileSystemFromVolume(ctx, volume)
			if fsErr != nil {
				Logc(ctx).WithError(fsErr).Errorf("Internal error creating filesystem.")
				return nil, fsErr
			}
			filesystems = append(filesystems, filesystem)
		}
	}

	if c.config.DebugTraceFlags["api"] {
		Logc(ctx).WithFields(logFields).Debug("Read volumes from capacity pool.")
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
		return nil, errors.NotFoundError("volume with creation token '%s' not found", creationToken)
	case 1:
		return matchingFilesystems[0], nil
	default:
		return nil, errors.NotFoundError("multiple volumes with creation token '%s' found", creationToken)
	}
}

// VolumeExistsByCreationToken checks whether a volume exists using its creation token as a key.
func (c Client) VolumeExistsByCreationToken(ctx context.Context, creationToken string) (bool, *FileSystem, error) {
	if filesystem, err := c.VolumeByCreationToken(ctx, creationToken); err != nil {
		if errors.IsNotFoundError(err) {
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
	logFields := LogFields{
		"API": "VolumesClient.Get",
		"ID":  id,
	}

	Logc(ctx).WithFields(logFields).Trace("Fetching volume by ID.")

	_, resourceGroup, _, netappAccount, cPoolName, volumeName, err := ParseVolumeID(id)
	if err != nil {
		return nil, err
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	response, err := c.sdkClient.VolumesClient.Get(responseCtx,
		resourceGroup, netappAccount, cPoolName, volumeName, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Volume not found.")
			return nil, errors.NotFoundError("volume with ID '%s' not found", id)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Found volume by ID.")

	return c.newFileSystemFromVolume(ctx, &response.Volume)
}

// VolumeExistsByID checks whether a volume exists using its creation token as a key.
func (c Client) VolumeExistsByID(ctx context.Context, id string) (bool, *FileSystem, error) {
	if filesystem, err := c.VolumeByID(ctx, id); err != nil {
		if errors.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	} else {
		return true, filesystem, nil
	}
}

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
			if desiredState == StateDeleted && errors.IsNotFoundError(err) {
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

		errMsg := fmt.Sprintf("volume state is %s, not %s", f.ProvisioningState, desiredState)
		if desiredState == StateDeleted && f.ProvisioningState == StateDeleting {
			err = errors.VolumeDeletingError(errMsg)
		} else {
			err = errors.New(errMsg)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		if utils.SliceContainsString(abortStates, f.ProvisioningState) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
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
			Logc(ctx).Warningf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return volumeState, nil
}

// CreateVolume creates a new volume.
func (c Client) CreateVolume(ctx context.Context, request *FilesystemCreateRequest) (*FileSystem, error) {
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

	serviceLevel := netapp.ServiceLevel(cPool.ServiceLevel)
	networkFeatures := netapp.NetworkFeatures(request.NetworkFeatures)

	newVol := netapp.Volume{
		Location: &location,
		Name:     &request.Name,
		Tags:     tags,
		Properties: &netapp.VolumeProperties{
			CreationToken:            &request.CreationToken,
			ServiceLevel:             &serviceLevel,
			UsageThreshold:           &request.QuotaInBytes,
			ExportPolicy:             exportPolicyExport(&request.ExportPolicy),
			ProtocolTypes:            CreateStringPtrArray(request.ProtocolTypes),
			SubnetID:                 &request.SubnetID,
			SnapshotDirectoryVisible: &request.SnapshotDirectory,
			NetworkFeatures:          &networkFeatures,
		},
	}

	// Only set the snapshot ID if we are cloning
	if request.SnapshotID != "" {
		newVol.Properties.SnapshotID = &request.SnapshotID
	}

	// Only send unix permissions if specified, since it is not yet a GA feature
	if request.UnixPermissions != "" {
		newVol.Properties.UnixPermissions = &request.UnixPermissions
	}

	Logc(ctx).WithFields(LogFields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"resourceGroup": resourceGroup,
		"netAppAccount": netappAccount,
		"capacityPool":  cPoolName,
		"subnetID":      request.SubnetID,
		"snapshotID":    request.SnapshotID,
		"snapshotDir":   request.SnapshotDirectory,
	}).Debug("Issuing create request.")

	logFields := LogFields{
		"API":           "VolumesClient.BeginCreateOrUpdate",
		"volume":        volumeFullName,
		"creationToken": request.CreationToken,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	_, err := c.sdkClient.VolumesClient.BeginCreateOrUpdate(responseCtx,
		resourceGroup, netappAccount, cPoolName, request.Name, newVol, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating volume.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Volume create request issued.")

	// The volume doesn't exist yet, so forge the volume ID to enable conversion to a FileSystem struct
	newVolID := CreateVolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, cPoolName, request.Name)
	newVol.ID = &newVolID

	return c.newFileSystemFromVolume(ctx, &newVol)
}

// ModifyVolume updates attributes of a volume.
func (c Client) ModifyVolume(
	ctx context.Context, filesystem *FileSystem, labels map[string]string, unixPermissions *string,
) error {
	logFields := LogFields{
		"API":    "VolumesClient.Get",
		"volume": filesystem.FullName,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	// Fetch the netapp.Volume to fill in the updated fields
	response, err := c.sdkClient.VolumesClient.Get(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error finding volume to modify.")
		return fmt.Errorf("couldn't get volume to modify; %v", err)
	}
	anfVolume := response.Volume

	Logc(ctx).WithFields(logFields).Debug("Found volume to modify.")

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
		anfVolume.Properties.UnixPermissions = unixPermissions
	}

	// Clear out ReadOnly and other fields that we don't want to change when merely relabeling.
	serviceLevel := netapp.ServiceLevel("")
	anfVolume.Properties.ServiceLevel = &serviceLevel
	anfVolume.Properties.ProvisioningState = nil
	anfVolume.Properties.ExportPolicy = nil
	anfVolume.Properties.ProtocolTypes = nil
	anfVolume.Properties.MountTargets = nil
	anfVolume.Properties.ThroughputMibps = nil
	anfVolume.Properties.BaremetalTenantID = nil

	Logc(ctx).WithFields(LogFields{
		"name":          anfVolume.Name,
		"creationToken": anfVolume.Properties.CreationToken,
	}).Debug("Modifying volume.")

	logFields = LogFields{
		"API":    "VolumesClient.BeginCreateOrUpdate",
		"volume": filesystem.FullName,
	}

	_, err = c.sdkClient.VolumesClient.BeginCreateOrUpdate(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, anfVolume, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error modifying volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume modified.")

	return nil
}

// ResizeVolume sends a VolumePatch to update a volume's quota.
func (c Client) ResizeVolume(ctx context.Context, filesystem *FileSystem, newSizeBytes int64) error {
	logFields := LogFields{
		"API":    "VolumesClient.BeginUpdate",
		"volume": filesystem.FullName,
	}

	patch := netapp.VolumePatch{
		ID:       &filesystem.ID,
		Location: &filesystem.Location,
		Name:     &filesystem.Name,
		Properties: &netapp.VolumePatchProperties{
			UsageThreshold: &newSizeBytes,
		},
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := c.sdkClient.VolumesClient.BeginUpdate(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, patch, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error resizing volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume resize request issued.")

	_, err = poller.PollUntilDone(responseCtx, &runtime.PollUntilDoneOptions{Frequency: 2 * time.Second})
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error polling for volume resize result.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume resize complete.")

	return nil
}

// DeleteVolume deletes a volume.
func (c Client) DeleteVolume(ctx context.Context, filesystem *FileSystem) error {
	logFields := LogFields{
		"API":    "VolumesClient.BeginDelete",
		"volume": filesystem.FullName,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	_, err := c.sdkClient.VolumesClient.BeginDelete(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Volume already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume deleted.")

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

	if anfSnapshot.Properties == nil {
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
		SnapshotID:        DerefString(anfSnapshot.Properties.SnapshotID),
		ProvisioningState: DerefString(anfSnapshot.Properties.ProvisioningState),
	}

	if anfSnapshot.Properties.Created != nil {
		snapshot.Created = *anfSnapshot.Properties.Created
	}

	return &snapshot, nil
}

// SnapshotsForVolume returns a list of snapshots on a volume.
func (c Client) SnapshotsForVolume(ctx context.Context, filesystem *FileSystem) (*[]*Snapshot, error) {
	logFields := LogFields{
		"API":    "SnapshotsClient.NewListPager",
		"volume": filesystem.FullName,
	}

	var snapshots []*Snapshot

	pager := c.sdkClient.SnapshotsClient.NewListPager(
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, nil)

	for pager.More() {
		var rawResponse *http.Response
		responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

		nextResult, err := pager.NextPage(responseCtx)

		logFields["correlationID"] = GetCorrelationID(rawResponse)

		if err != nil {
			Logc(ctx).WithFields(logFields).Error("Could not iterate snapshots.")
			return nil, fmt.Errorf("error iterating snapshots; %v", err)
		}

		for _, anfSnapshot := range nextResult.Value {
			snapshot, snapErr := c.newSnapshotFromANFSnapshot(ctx, anfSnapshot)
			if snapErr != nil {
				Logc(ctx).WithError(snapErr).Errorf("Internal error creating snapshot.")
				return nil, snapErr
			}
			snapshots = append(snapshots, snapshot)
		}
	}

	Logc(ctx).WithFields(logFields).Debug("Read snapshots from volume.")

	return &snapshots, nil
}

// SnapshotForVolume fetches a specific snapshot on a volume by its name.
func (c Client) SnapshotForVolume(
	ctx context.Context, filesystem *FileSystem, snapshotName string,
) (*Snapshot, error) {
	logFields := LogFields{
		"API":      "SnapshotsClient.Get",
		"volume":   filesystem.FullName,
		"snapshot": snapshotName,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	response, err := c.sdkClient.SnapshotsClient.Get(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool,
		filesystem.Name, snapshotName, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Snapshot not found.")
			return nil, errors.NotFoundError("snapshot %s not found", snapshotName)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching snapshot.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Found snapshot.")

	return c.newSnapshotFromANFSnapshot(ctx, &response.Snapshot)
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
			if desiredState == StateDeleted && errors.IsNotFoundError(err) {
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
		Logc(ctx).WithFields(LogFields{
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
			Logc(ctx).Warningf("Snapshot state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debugf("Desired snapshot state reached.")

	return nil
}

// CreateSnapshot creates a new snapshot.
func (c Client) CreateSnapshot(ctx context.Context, filesystem *FileSystem, name string) (*Snapshot, error) {
	logFields := LogFields{
		"API":      "SnapshotsClient.BeginCreate",
		"volume":   filesystem.FullName,
		"snapshot": name,
	}

	anfSnapshot := netapp.Snapshot{
		Location: &filesystem.Location,
		Name:     &name,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	// Create the snapshot
	_, err := c.sdkClient.SnapshotsClient.BeginCreate(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool,
		filesystem.Name, name, anfSnapshot, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating snapshot.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Snapshot create request issued.")

	// The snapshot doesn't exist yet, so forge the snapshot ID to enable conversion to a Snapshot struct
	newSnapshotID := CreateSnapshotID(c.config.SubscriptionID, filesystem.ResourceGroup,
		filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, name)
	anfSnapshot.ID = &newSnapshotID

	anfSnapshot.Properties = &netapp.SnapshotProperties{}

	return c.newSnapshotFromANFSnapshot(ctx, &anfSnapshot)
}

// RestoreSnapshot restores a volume to a snapshot.
func (c Client) RestoreSnapshot(ctx context.Context, filesystem *FileSystem, snapshot *Snapshot) error {
	logFields := LogFields{
		"API":      "SnapshotsClient.BeginRevert",
		"volume":   filesystem.FullName,
		"snapshot": snapshot.Name,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	revertBody := netapp.VolumeRevert{
		SnapshotID: utils.Ptr(snapshot.SnapshotID),
	}

	_, err := c.sdkClient.VolumesClient.BeginRevert(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool,
		filesystem.Name, revertBody, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error reverting snapshot.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume reverted to snapshot.")

	return nil
}

// DeleteSnapshot deletes a snapshot.
func (c Client) DeleteSnapshot(ctx context.Context, filesystem *FileSystem, snapshot *Snapshot) error {
	logFields := LogFields{
		"API":      "SnapshotsClient.BeginDelete",
		"volume":   filesystem.FullName,
		"snapshot": snapshot.Name,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	_, err := c.sdkClient.SnapshotsClient.BeginDelete(responseCtx,
		filesystem.ResourceGroup, filesystem.NetAppAccount, filesystem.CapacityPool,
		filesystem.Name, snapshot.Name, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Snapshot already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Snapshot deleted.")

	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to retrieve and manage subvolumes
// ///////////////////////////////////////////////////////////////////////////////

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

	if subVol.Properties == nil {
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
		ProvisioningState: DerefString(subVol.Properties.ProvisioningState),
		Size:              DerefInt64(subVol.Properties.Size),
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

	if subVolModel.Properties == nil {
		return nil, fmt.Errorf("subvolume model %s has no properties", *subVolModel.Name)
	}

	original.ProvisioningState = DerefString(subVolModel.Properties.ProvisioningState)
	original.Size = DerefInt64(subVolModel.Properties.Size)

	if subVolModel.Properties.CreationTimeStamp != nil {
		original.Created = *subVolModel.Properties.CreationTimeStamp
	}

	return original, nil
}

// SubvolumesForVolume returns a list of subvolume on a volume.
func (c Client) SubvolumesForVolume(ctx context.Context, filesystem *FileSystem) (*[]*Subvolume, error) {
	logFields := LogFields{
		"API":    "SubvolumesClient.NewListByVolumePager",
		"volume": filesystem.FullName,
	}

	var subvolumes []*Subvolume

	pager := c.sdkClient.SubvolumesClient.NewListByVolumePager(filesystem.ResourceGroup,
		filesystem.NetAppAccount, filesystem.CapacityPool, filesystem.Name, nil)

	for pager.More() {
		var rawResponse *http.Response
		responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

		nextResult, err := pager.NextPage(responseCtx)

		logFields["correlationID"] = GetCorrelationID(rawResponse)

		if err != nil {
			Logc(ctx).WithFields(logFields).Error("Could not iterate subvolumes.")
			return nil, fmt.Errorf("error iterating subvolumes: %v", err)
		}

		for _, anfSubvolume := range nextResult.Value {
			subvolume, subvolumeErr := c.newSubvolumeFromSubvolumeInfo(ctx, anfSubvolume)
			if subvolumeErr != nil {
				Logc(ctx).WithError(subvolumeErr).Errorf("Internal error creating subvolume.")
				return nil, subvolumeErr
			}
			subvolumes = append(subvolumes, subvolume)
		}
	}

	Logc(ctx).WithFields(logFields).Debug("Read subvolumes from volume.")

	return &subvolumes, nil
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

		volume, err := c.VolumeByID(ctx,
			CreateVolumeID(subscriptionID, resourceGroup, netappAccount, cPoolName, volumeName))
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
	// When we know the internal ID, use that as it is vastly more efficient.
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
		if errors.IsNotFoundError(err) {
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
			if errors.IsNotFoundError(err) {
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
		return nil, errors.NotFoundError("subvolume with creation token '%s' not found", creationToken)
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
		return nil, errors.NotFoundError("multiple subvolumes with creation token '%s' found",
			creationToken)
	}
}

// SubvolumeByID returns a Subvolume based on its Azure-style ID.
func (c Client) SubvolumeByID(ctx context.Context, subvolumeID string, queryMetadata bool) (*Subvolume, error) {
	logFields := LogFields{
		"API": "SubvolumesClient.Get",
		"ID":  subvolumeID,
	}

	_, resourceGroup, _, netappAccount, capacityPool, volumeName, subvolumeName, err := ParseSubvolumeID(subvolumeID)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Trace("Fetching subvolume by ID.")

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	response, err := c.sdkClient.SubvolumesClient.Get(responseCtx,
		resourceGroup, netappAccount, capacityPool, volumeName, subvolumeName, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Subvolume not found.")
			return nil, errors.NotFoundError("subvolume with ID '%s' not found", subvolumeID)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching subvolume.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Subvolume found by ID")

	subvolume, err := c.newSubvolumeFromSubvolumeInfo(ctx, &response.SubvolumeInfo)
	if err != nil {
		return nil, err
	}

	// GET does not fetch metadata as it requires talking to actual storage and the delay exceeds Azure 1 second time
	// limit.
	if queryMetadata {
		Logc(ctx).Debugf("Fetching subvolume %s metadata.", subvolumeID)
		return c.SubvolumeMetadata(ctx, subvolume)
	} else {
		return subvolume, nil
	}
}

// SubvolumeMetadata fetches a Subvolume metadata, melds it into the supplied subvolume, and returns the result.
func (c Client) SubvolumeMetadata(ctx context.Context, subvolume *Subvolume) (*Subvolume, error) {
	logFields := LogFields{
		"API": "SubvolumesClient.BeginGetMetadata",
		"ID":  subvolume.ID,
	}

	Logc(ctx).WithFields(logFields).Tracef("Fetching subvolume metadata.")

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := c.sdkClient.SubvolumesClient.BeginGetMetadata(responseCtx, subvolume.ResourceGroup,
		subvolume.NetAppAccount, subvolume.CapacityPool, subvolume.Volume, subvolume.Name, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Subvolume not found.")
			return nil, errors.NotFoundError("subvolume with ID '%s' not found", subvolume.ID)
		}
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching subvolume metadata.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Waiting for the request to complete.")

	// Create a context with timeout as needed by PollUntilDone()
	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	responseCtx = runtime.WithCaptureResponse(sdkCtx, &rawResponse)
	defer cancel()

	response, err := poller.PollUntilDone(responseCtx, &runtime.PollUntilDoneOptions{Frequency: 2 * time.Second})

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error polling for subvolume metadata.")
		return nil, err
	}

	return c.updateSubvolumeFromSubvolumeModel(ctx, subvolume, &response.SubvolumeModel)
}

// SubvolumeExistsByID checks whether a subvolume exists using its creation token as a key.
func (c Client) SubvolumeExistsByID(ctx context.Context, id string) (bool, *Subvolume, error) {
	if subvolume, err := c.SubvolumeByID(ctx, id, false); err != nil {
		if errors.IsNotFoundError(err) {
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

	logFields := LogFields{
		"ID":           subvolume.ID,
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
				if errors.IsNotFoundError(err) {
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
		Logc(ctx).WithFields(LogFields{
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
			Logc(ctx).WithFields(logFields).Warningf("Subvolume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return subvolumeState, err
	}

	Logc(ctx).WithFields(logFields).Debug("Desired subvolume state reached.")

	return subvolumeState, nil
}

// CreateSubvolume creates a new subvolume
func (c Client) CreateSubvolume(ctx context.Context, request *SubvolumeCreateRequest) (*Subvolume, PollerResponse, error) {
	subvolumeName := request.CreationToken

	resourceGroup, netappAccount, cpoolName, volumeName, err := ParseVolumeName(request.Volume)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get volume information: %v", err)
	}

	path := "/" + subvolumeName
	newSubvol := netapp.SubvolumeInfo{
		Name: &subvolumeName,
		Properties: &netapp.SubvolumeProperties{
			Path: &path,
		},
	}

	if request.Size > 0 {
		newSubvol.Properties.Size = &request.Size
	}

	if request.Parent != "" {
		parentPath := "/" + request.Parent
		newSubvol.Properties.ParentPath = &parentPath
	}

	Logc(ctx).WithFields(LogFields{
		"name":          subvolumeName,
		"resourceGroup": resourceGroup,
		"netAppAccount": netappAccount,
		"capacityPool":  cpoolName,
		"volumeName":    volumeName,
	}).Debug("Issuing subvolume create request.")

	logFields := LogFields{
		"API":    "SubvolumesClient.BeginCreate",
		"volume": volumeName,
		"name":   subvolumeName,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := c.sdkClient.SubvolumesClient.BeginCreate(responseCtx,
		resourceGroup, netappAccount, cpoolName, volumeName, subvolumeName, newSubvol, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating subvolume.")
		return nil, nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Subvolume create request issued.")

	// The subvolume doesn't exist yet, so forge the subvolume ID to enable conversion to a Subvolume struct
	newSubvolumeID := CreateSubvolumeID(c.config.SubscriptionID, resourceGroup, netappAccount, cpoolName, volumeName,
		request.CreationToken)
	newSubvol.ID = &newSubvolumeID

	subvol, err := c.newSubvolumeFromSubvolumeInfo(ctx, &newSubvol)
	createPollerResp := PollerSVCreateResponse{poller}

	return subvol, &createPollerResp, err
}

// ResizeSubvolume sends a SubvolumePatchRequest to update a subvolume's size.
func (c Client) ResizeSubvolume(ctx context.Context, subvolume *Subvolume, newSizeBytes int64) error {
	logFields := LogFields{
		"API": "SubvolumesClient.BeginUpdate",
		"ID":  subvolume.ID,
	}

	path := "/" + subvolume.Name
	patch := &netapp.SubvolumePatchRequest{
		Properties: &netapp.SubvolumePatchParams{
			Size: &newSizeBytes,
			Path: &path,
		},
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := c.sdkClient.SubvolumesClient.BeginUpdate(responseCtx,
		subvolume.ResourceGroup, subvolume.NetAppAccount, subvolume.CapacityPool,
		subvolume.Volume, subvolume.Name, *patch, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error resizing subvolume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Subvolume resize request issued.")

	_, err = poller.PollUntilDone(responseCtx, &runtime.PollUntilDoneOptions{Frequency: 2 * time.Second})
	if err != nil {
		trimmedErr := GetMessageFromError(responseCtx, err)
		Logc(ctx).WithFields(logFields).WithError(trimmedErr).Error("Error polling for subvolume resize result.")
		return trimmedErr
	}

	Logc(ctx).WithFields(logFields).Debug("Subvolume resize complete.")

	return nil
}

// DeleteSubvolume deletes a subvolume.
func (c Client) DeleteSubvolume(ctx context.Context, subvolume *Subvolume) (PollerResponse, error) {
	logFields := LogFields{
		"API": "SubvolumesClient.BeginDelete",
		"ID":  subvolume.ID,
	}

	var rawResponse *http.Response
	responseCtx := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := c.sdkClient.SubvolumesClient.BeginDelete(responseCtx,
		subvolume.ResourceGroup, subvolume.NetAppAccount, subvolume.CapacityPool,
		subvolume.Volume, subvolume.Name, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Subvolume already deleted.")
			return nil, nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting subvolume.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Subvolume deleted.")
	deletePollerResp := PollerSVDeleteResponse{poller}

	return &deletePollerResp, nil
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

	if detailedErr, ok := err.(*azcore.ResponseError); ok {
		if detailedErr.RawResponse != nil && detailedErr.RawResponse.StatusCode == http.StatusNotFound {
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

	if detailedErr, ok := err.(*azcore.ResponseError); ok {
		if detailedErr.RawResponse != nil && detailedErr.RawResponse.StatusCode == http.StatusTooManyRequests {
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

	if detailedErr, ok := err.(*azcore.ResponseError); ok {
		id = GetCorrelationID(detailedErr.RawResponse)
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

// GetMessageFromError accepts an error returned from the ANF SDK and extracts
// the error message.
func GetMessageFromError(ctx context.Context, inputErr error) error {
	type AzureError struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if inputErr == nil {
		return inputErr
	}

	if detailedErr, ok := inputErr.(*azcore.ResponseError); ok {
		if detailedErr.RawResponse != nil && detailedErr.RawResponse.Body != nil {
			bytes, readErr := io.ReadAll(detailedErr.RawResponse.Body)
			if readErr != nil {
				Logc(ctx).WithError(readErr).Error("Failed to read error body.")
				return inputErr
			}

			var azureError AzureError
			unmarshalErr := json.Unmarshal(bytes, &azureError)
			if unmarshalErr != nil {
				Logc(ctx).WithError(unmarshalErr).Error("Unmarshal error.")
			} else {
				errCode := azureError.Error.Code
				errMessage := azureError.Error.Message

				if errMessage != "" {
					return fmt.Errorf("%v: %v", errCode, errMessage)
				}
			}
		}
	}

	return inputErr
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

// DerefStringPtrArray accepts an array of string pointers and returns the value of the array.
func DerefStringPtrArray(in []*string) []string {
	out := make([]string, len(in))
	for index, sPtr := range in {
		out[index] = DerefString(sPtr)
	}
	return out
}

// CreateStringPtrArray accepts an array of strings and returns an array of pointers to those strings.
func CreateStringPtrArray(in []string) []*string {
	out := make([]*string, 0)
	if in != nil {
		for index := range in {
			s := in[index]
			out = append(out, &s)
		}
	}
	return out
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

// DerefNetworkFeatures accepts a NetworkFeatures pointer and returns its string value, or "" if the pointer is nil.
func DerefNetworkFeatures(f *netapp.NetworkFeatures) string {
	if f != nil {
		return string(*f)
	}
	return ""
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
