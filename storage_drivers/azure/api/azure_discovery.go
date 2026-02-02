// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Azure NetApp Files SDK
package api

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	resourcegraph "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	features "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armfeatures"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"

	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

const (
	serviceLevel       = "serviceLevel"
	virtualNetwork     = "virtualNetwork"
	subnet             = "subnet"
	resourceGroups     = "resourceGroups"
	netappAccounts     = "netappAccounts"
	capacityPools      = "capacityPools"
	qosType            = "qosType"
	DefaultMaxCacheAge = 10 * time.Minute
)

// ///////////////////////////////////////////////////////////////////////////////
// Top level discovery functions
// ///////////////////////////////////////////////////////////////////////////////

// RefreshAzureResources refreshes the cache of discovered Azure resources and validates
// them against our known storage pools.
func (c Client) RefreshAzureResources(ctx context.Context) error {
	// Check if it is time to update the cache
	if time.Now().Before(c.sdkClient.AzureResources.lastUpdateTime.Add(c.config.MaxCacheAge)) {
		Logc(ctx).Debugf("Cached resources not yet %v old, skipping refresh.", c.config.MaxCacheAge)
		return nil
	}

	// (re-)Discover what we have to work with in Azure
	Logc(ctx).Debugf("Discovering Azure resources.")
	discoveryErr := multierr.Combine(c.DiscoverAzureResources(ctx))

	// This is noisy, hide it behind api tracing.
	c.dumpAzureResources(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"])

	// Warn about anything in the config that doesn't match any discovered resources
	c.checkForNonexistentResourceGroups(ctx)
	c.checkForNonexistentNetAppAccounts(ctx)
	c.checkForNonexistentCapacityPools(ctx)
	c.checkForNonexistentVirtualNetworks(ctx)
	c.checkForNonexistentSubnets(ctx)

	// Return errors for any storage pool that cannot be satisfied by discovered resources
	poolErrors := multierr.Combine(c.checkForUnsatisfiedPools(ctx)...)
	discoveryErr = multierr.Combine(discoveryErr, poolErrors)

	Logc(ctx).Debugf("Discovering Azure preview features.")
	discoveryErr = multierr.Combine(discoveryErr, c.EnableAzureFeatures(ctx, FeatureUnixPermissions))

	return discoveryErr
}

// DiscoverAzureResources rediscovers the Azure resources we care about and updates the cache.
func (c Client) DiscoverAzureResources(ctx context.Context) (returnError error) {
	// Start from scratch each time we are called.  All discovered resources are nested under ResourceGroups.
	newResourceGroups := make([]*ResourceGroup, 0)
	newResourceGroupMap := make(map[string]*ResourceGroup)
	newNetAppAccountMap := make(map[string]*NetAppAccount)
	newCapacityPoolMap := make(map[string]*CapacityPool)
	newVirtualNetworkMap := make(map[string]*VirtualNetwork)
	newSubnetMap := make(map[string]*Subnet)

	defer func() {
		if returnError != nil {
			Logc(ctx).WithError(returnError).Debug("Discovery error, not retaining any discovered resources.")
			return
		}

		// Swap the newly discovered resources into the cache only if discovery succeeded.
		c.sdkClient.AzureResources.ResourceGroups = newResourceGroups
		c.sdkClient.AzureResources.ResourceGroupMap = newResourceGroupMap
		c.sdkClient.AzureResources.NetAppAccountMap = newNetAppAccountMap
		c.sdkClient.AzureResources.CapacityPoolMap = newCapacityPoolMap
		c.sdkClient.AzureResources.VirtualNetworkMap = newVirtualNetworkMap
		c.sdkClient.AzureResources.SubnetMap = newSubnetMap
		c.sdkClient.AzureResources.lastUpdateTime = time.Now()

		Logc(ctx).Debug("Switched to newly discovered resources.")
	}()

	// Discover capacity pools
	cPools, returnError := c.discoverCapacityPoolsWithRetry(ctx)
	if returnError != nil {
		return returnError
	}

	// Update maps with all data from discovered capacity pools
	for _, cPool := range *cPools {

		var rg *ResourceGroup
		var naa *NetAppAccount
		var ok bool

		// Create resource group if not found yet
		if rg, ok = newResourceGroupMap[cPool.ResourceGroup]; !ok {
			rg = &ResourceGroup{
				Name:            cPool.ResourceGroup,
				NetAppAccounts:  make([]*NetAppAccount, 0),
				VirtualNetworks: make([]*VirtualNetwork, 0),
			}
			newResourceGroups = append(newResourceGroups, rg)
			newResourceGroupMap[cPool.ResourceGroup] = rg
		}

		naaFullName := CreateNetappAccountFullName(cPool.ResourceGroup, cPool.NetAppAccount)

		// Create netapp account if not found yet
		if naa, ok = newNetAppAccountMap[naaFullName]; !ok {
			naa = &NetAppAccount{
				ID:            CreateNetappAccountID(c.config.SubscriptionID, cPool.ResourceGroup, cPool.NetAppAccount),
				ResourceGroup: cPool.ResourceGroup,
				Name:          cPool.NetAppAccount,
				FullName:      naaFullName,
				Location:      cPool.Location,
				Type:          "Microsoft.NetApp/netAppAccounts",
				CapacityPools: make([]*CapacityPool, 0),
			}
			newNetAppAccountMap[naaFullName] = naa
			rg.NetAppAccounts = append(rg.NetAppAccounts, naa)
		}

		// Add capacity pool to account
		naa.CapacityPools = append(naa.CapacityPools, cPool)
		newCapacityPoolMap[cPool.FullName] = cPool
	}

	// Discover ANF-delegated subnets
	subnets, returnError := c.discoverSubnetsWithRetry(ctx)
	if returnError != nil {
		return returnError
	}

	// Update maps with all data from discovered subnets
	for _, subnet := range *subnets {

		var rg *ResourceGroup
		var vnet *VirtualNetwork
		var ok bool

		// Create resource group if not found yet
		if rg, ok = newResourceGroupMap[subnet.ResourceGroup]; !ok {
			rg = &ResourceGroup{
				Name:            subnet.ResourceGroup,
				NetAppAccounts:  make([]*NetAppAccount, 0),
				VirtualNetworks: make([]*VirtualNetwork, 0),
			}
			newResourceGroups = append(newResourceGroups, rg)
			newResourceGroupMap[subnet.ResourceGroup] = rg
		}

		vnetFullName := CreateVirtualNetworkFullName(subnet.ResourceGroup, subnet.VirtualNetwork)

		// Create virtual network if not found yet
		if vnet, ok = newVirtualNetworkMap[vnetFullName]; !ok {
			vnet = &VirtualNetwork{
				ID:            CreateVirtualNetworkID(c.config.SubscriptionID, subnet.ResourceGroup, subnet.VirtualNetwork),
				ResourceGroup: subnet.ResourceGroup,
				Name:          subnet.VirtualNetwork,
				FullName:      vnetFullName,
				Location:      subnet.Location,
				Type:          "Microsoft.Network/virtualNetworks",
				Subnets:       make([]*Subnet, 0),
			}
			newVirtualNetworkMap[vnetFullName] = vnet
			rg.VirtualNetworks = append(rg.VirtualNetworks, vnet)
		}

		// Add subnet to virtual network
		vnet.Subnets = append(vnet.Subnets, subnet)
		newSubnetMap[subnet.FullName] = subnet
	}

	// Detect the lack of any resources: can occur when no connectivity, etc.
	// Would like a better way of proactively finding out there is something wrong
	// at a very basic level.  (Reproduce this by turning off your network!)
	numResourceGroups := len(newResourceGroupMap)
	numCapacityPools := len(newCapacityPoolMap)
	numSubnets := len(newSubnetMap)

	if numResourceGroups == 0 {
		return errors.New("no resource groups discovered; check connectivity, credentials")
	}
	if numCapacityPools == 0 {
		return errors.New("no capacity pools discovered; volume provisioning may fail until corrected")
	}
	if numSubnets == 0 {
		return errors.New("no ANF subnets discovered; volume provisioning may fail until corrected")
	}

	Logc(ctx).WithFields(LogFields{
		"resourceGroups": numResourceGroups,
		"capacityPools":  numCapacityPools,
		"subnets":        numSubnets,
	}).Info("Discovered Azure resources.")

	return returnError
}

// dumpAzureResources writes a hierarchical representation of discovered resources to the log.
func (c Client) dumpAzureResources(ctx context.Context, driverName string, discoveryTraceEnabled bool) {
	Logd(ctx, driverName, discoveryTraceEnabled).Tracef("Discovered Azure Resources:")
	for _, rg := range c.sdkClient.AzureResources.ResourceGroups {
		Logd(ctx, driverName, discoveryTraceEnabled).Tracef("  Resource Group: %s", rg.Name)
		for _, na := range rg.NetAppAccounts {
			Logd(ctx, driverName, discoveryTraceEnabled).Tracef("    ANF Account: %s, Location: %s", na.Name, na.Location)
			for _, cp := range na.CapacityPools {
				Logd(ctx, driverName, discoveryTraceEnabled).Tracef("      CPool: %s, [%s, %s]", cp.Name, cp.ServiceLevel, cp.QosType)
			}
		}
		for _, vn := range rg.VirtualNetworks {
			Logd(ctx, driverName, discoveryTraceEnabled).Tracef("    Network: %s, Location: %s", vn.Name, vn.Location)
			for _, sn := range vn.Subnets {
				Logd(ctx, driverName, discoveryTraceEnabled).Tracef("      Subnet: %s", sn.Name)
			}
		}
	}
}

// checkForUnsatisfiedPools returns one or more errors if one or more configured storage pools
// are satisfied by no capacity pools.
func (c Client) checkForUnsatisfiedPools(ctx context.Context) (discoveryErrors []error) {
	// Ensure every storage pool matches one or more capacity pools
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Find all capacity pools that work for this storage pool
		cPools := c.CapacityPoolsForStoragePool(ctx, sPool,
			sPool.InternalAttributes()[serviceLevel], sPool.InternalAttributes()[qosType])

		if len(cPools) == 0 {

			err := fmt.Errorf("no capacity pools found for storage pool %s", sPoolName)
			Logc(ctx).WithError(err).Error("Discovery error.")
			discoveryErrors = append(discoveryErrors, err)

		} else {

			cPoolFullNames := make([]string, 0)
			for _, cPool := range cPools {
				cPoolFullNames = append(cPoolFullNames, cPool.FullName)
			}

			// Print the mapping in the logs so we see it after each discovery refresh.
			Logc(ctx).Debugf("Storage pool %s mapped to capacity pools %v.", sPoolName, cPoolFullNames)
		}
	}

	return
}

// checkForNonexistentResourceGroups logs warnings if any configured resource groups do not
// match discovered resource groups in the resource cache.
func (c Client) checkForNonexistentResourceGroups(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Build list of resource group names
		rgNames := make([]string, 0)
		for _, cacheRG := range c.sdkClient.AzureResources.ResourceGroupMap {
			rgNames = append(rgNames, cacheRG.Name)
		}

		// Find any resource group value in this storage pool that doesn't match known resource groups
		for _, configRG := range collection.SplitString(ctx, sPool.InternalAttributes()[resourceGroups], ",") {
			if !collection.StringInSlice(configRG, rgNames) {
				anyMismatches = true

				Logc(ctx).WithFields(LogFields{
					"pool":          sPoolName,
					"resourceGroup": configRG,
				}).Warning("Resource group referenced in pool not found.")
			}
		}
	}

	return
}

// checkForNonexistentNetAppAccounts logs warnings if any configured NetApp accounts do not
// match discovered NetApp accounts in the resource cache.
func (c Client) checkForNonexistentNetAppAccounts(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Build list of short and long netapp account names
		naNames := make([]string, 0)
		for _, cacheNA := range c.sdkClient.AzureResources.NetAppAccountMap {
			naNames = append(naNames, cacheNA.Name)
			naNames = append(naNames, cacheNA.FullName)
		}

		// Find any netapp account value in this storage pool that doesn't match known netapp accounts
		for _, configNA := range collection.SplitString(ctx, sPool.InternalAttributes()[netappAccounts], ",") {
			if !collection.StringInSlice(configNA, naNames) {
				anyMismatches = true

				Logc(ctx).WithFields(LogFields{
					"pool":          sPoolName,
					"netappAccount": configNA,
				}).Warning("NetApp account referenced in pool not found.")
			}
		}
	}

	return
}

// checkForNonexistentCapacityPools logs warnings if any configured capacity pools do not
// match discovered capacity pools in the resource cache.
func (c Client) checkForNonexistentCapacityPools(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Build list of short and long capacity pool names
		cpNames := make([]string, 0)
		for _, cacheCP := range c.sdkClient.AzureResources.CapacityPoolMap {
			cpNames = append(cpNames, cacheCP.Name)
			cpNames = append(cpNames, cacheCP.FullName)
		}

		// Find any capacity pools value in this storage pool that doesn't match known capacity pools
		for _, configCP := range collection.SplitString(ctx, sPool.InternalAttributes()[capacityPools], ",") {
			if !collection.StringInSlice(configCP, cpNames) {
				anyMismatches = true

				Logc(ctx).WithFields(LogFields{
					"pool":         sPoolName,
					"capacityPool": configCP,
				}).Warning("Capacity pool referenced in pool not found.")
			}
		}
	}

	return
}

// checkForNonexistentVirtualNetworks logs warnings if any configured virtual networks do not
// match discovered virtual networks in the resource cache.
func (c Client) checkForNonexistentVirtualNetworks(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Build list of short and long capacity virtual network names
		vnNames := make([]string, 0)
		for _, cacheVN := range c.sdkClient.AzureResources.VirtualNetworkMap {
			vnNames = append(vnNames, cacheVN.Name)
			vnNames = append(vnNames, cacheVN.FullName)
		}

		// Find any virtual network value in this storage pool that doesn't match known virtual networks
		configVN := sPool.InternalAttributes()[virtualNetwork]
		if configVN != "" && !collection.StringInSlice(configVN, vnNames) {
			anyMismatches = true

			Logc(ctx).WithFields(LogFields{
				"pool":           sPoolName,
				"virtualNetwork": configVN,
			}).Warning("Virtual network referenced in pool not found.")
		}
	}

	return
}

// checkForNonexistentSubnets logs warnings if any configured subnets do not
// match discovered subnets in the resource cache.
func (c Client) checkForNonexistentSubnets(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.AzureResources.StoragePoolMap {

		// Build list of short and long capacity subnet names
		snNames := make([]string, 0)
		for _, cacheSN := range c.sdkClient.AzureResources.SubnetMap {
			snNames = append(snNames, cacheSN.Name)
			snNames = append(snNames, cacheSN.FullName)
		}

		// Find any subnet value in this storage pool that doesn't match known subnets
		configSN := sPool.InternalAttributes()[subnet]
		if configSN != "" && !collection.StringInSlice(configSN, snNames) {
			anyMismatches = true

			Logc(ctx).WithFields(LogFields{
				"pool":   sPoolName,
				"subnet": configSN,
			}).Warning("Subnet referenced in pool not found.")
		}
	}

	return
}

// EnableAzureFeatures registers any ANF preview features we care about and updates the cache.
func (c Client) EnableAzureFeatures(ctx context.Context, featureNames ...string) (returnError error) {
	// Start from scratch each time we are called.  All discovered resources are nested under ResourceGroups.
	featureMap := make(map[string]bool)

	defer func() {
		if returnError != nil {
			Logc(ctx).WithError(returnError).Debug("Discovery error, not retaining any discovered resources.")
			return
		}

		// Swap the newly discovered features into the cache only if discovery succeeded.
		c.sdkClient.AzureResources.Features = featureMap

		Logc(ctx).Debug("Switched to newly discovered features.")
	}()

	returnError = multierr.Combine()

	for _, f := range featureNames {
		returnError = multierr.Combine(returnError, c.enableAzureFeature(ctx, "Microsoft.NetApp", f, featureMap))
	}

	return
}

// enableAzureFeature checks whether a specific Azure preview feature is Registered and modifies the supplied
// map with a value indicating the feature's availability.  If the API returns 404 (Not Found), this method assumes
// that the feature (which must have been a preview feature at one time) has graduated to GA status; the caller must
// know that the feature was not removed instead!
func (c Client) enableAzureFeature(
	ctx context.Context, provider, feature string, featureMap map[string]bool,
) (returnError error) {
	logFields := LogFields{"feature": feature}

	var rawResponse *http.Response
	responseCtx := policy.WithCaptureResponse(ctx, &rawResponse)

	result, err := c.sdkClient.FeaturesClient.Get(responseCtx, provider, feature, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		if IsANFNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Debug("Feature not found, assuming it is available.")
			featureMap[feature] = true
		} else {
			Logc(ctx).WithFields(logFields).WithError(err).Warning("Feature check failed, it will not be available.")
			featureMap[feature] = false
			returnError = err
		}
		return
	}

	if result.Properties == nil {
		Logc(ctx).WithFields(logFields).WithError(err).Warning("Invalid response, feature will not be available.")
		featureMap[feature] = false
		returnError = fmt.Errorf("invalid response while checking for %s feature", feature)
		return
	}

	featureState := features.SubscriptionFeatureRegistrationState(DerefString(result.Properties.State))
	featureMap[feature] = featureState == features.SubscriptionFeatureRegistrationStateRegistered

	Logc(ctx).WithFields(logFields).Debugf("Feature is %s.", featureState)

	// If feature is known to exist and not be registered, register it now.
	if featureState == features.SubscriptionFeatureRegistrationStateNotRegistered ||
		featureState == features.SubscriptionFeatureRegistrationStateUnregistered {

		_, registerError := c.sdkClient.FeaturesClient.Register(responseCtx, provider, feature, nil)

		logFields["correlationID"] = GetCorrelationID(rawResponse)

		if registerError != nil {
			Logc(ctx).WithFields(logFields).WithError(returnError).Warning("Could not register feature.")
		} else {
			Logc(ctx).WithFields(logFields).Debug("Registered feature.")
		}
	}

	return
}

// Features returns the map of preview features believed to be available in the current subscription.
func (c Client) Features() map[string]bool {
	featureMap := make(map[string]bool)
	for k, v := range c.sdkClient.Features {
		featureMap[k] = v
	}
	return featureMap
}

// HasFeature returns true if the named preview feature is believed to be available in the current subscription.
func (c Client) HasFeature(feature string) bool {
	value, ok := c.sdkClient.Features[feature]
	return ok && value
}

// ///////////////////////////////////////////////////////////////////////////////
// Internal functions to do discovery via the Azure Resource Graph APIs
// ///////////////////////////////////////////////////////////////////////////////

// discoverCapacityPoolsWithRetry queries the Azure Resource Graph for all ANF capacity pools in the current location,
// retrying if the API request is throttled.
func (c Client) discoverCapacityPoolsWithRetry(ctx context.Context) (pools *[]*CapacityPool, err error) {
	discover := func() error {
		if pools, err = c.discoverCapacityPools(ctx); err != nil && IsANFTooManyRequestsError(err) {
			return err
		}
		return backoff.Permanent(err)
	}

	notify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration.Truncate(10 * time.Millisecond),
		}).Debugf("Retrying capacity pools resource graph query.")
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = DefaultSDKTimeout
	expBackoff.MaxInterval = 5 * time.Second
	expBackoff.RandomizationFactor = 0.1
	expBackoff.InitialInterval = 5 * time.Second
	expBackoff.Multiplier = 1

	err = backoff.RetryNotify(discover, expBackoff, notify)

	return
}

// discoverCapacityPools queries the Azure Resource Graph for all ANF capacity pools in the current location.
func (c Client) discoverCapacityPools(ctx context.Context) (*[]*CapacityPool, error) {
	logFields := LogFields{
		"API": "GraphClient.Resources",
	}

	subscriptions := []string{c.config.SubscriptionID}
	query := fmt.Sprintf(`
    Resources
    | where type =~ 'Microsoft.NetApp/netAppAccounts/capacityPools' and location =~ '%s'`, c.config.Location)
	resultFormat := resourcegraph.ResultFormat("objectArray")
	requestOptions := resourcegraph.QueryRequestOptions{ResultFormat: &resultFormat}

	request := resourcegraph.QueryRequest{
		Subscriptions: CreateStringPtrArray(subscriptions),
		Query:         &query,
		Options:       &requestOptions,
	}

	var rawResponse *http.Response
	responseCtx := policy.WithCaptureResponse(ctx, &rawResponse)

	response, err := c.sdkClient.GraphClient.Resources(responseCtx, request, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Capacity pool query failed.")
		return nil, err
	}

	resourceList := response.QueryResponse

	Logc(ctx).WithFields(logFields).Debug("Read capacity pools from resource graph.")

	var cpools []*CapacityPool
	var data []interface{}
	var ok bool

	if resourceList.Data == nil || resourceList.Count == nil || *resourceList.Count == 0 {
		Logc(ctx).WithFields(logFields).Error("Capacity pool query returned no data.")
		return nil, errors.New("capacity pool query returned no data")
	} else if data, ok = resourceList.Data.([]interface{}); !ok {
		Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid data.")
		return nil, errors.New("capacity pool query returned invalid data")
	}

	for _, rawPoolInterface := range data {

		var rawPoolMap, rawProperties map[string]interface{}
		var id, resourceGroup, netappAccount, cPoolName, poolID, serviceLevel, provisioningState, qosType string

		if rawPoolMap, ok = rawPoolInterface.(map[string]interface{}); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned non-map data.")
			continue
		}

		if id, ok = rawPoolMap["id"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned non-string ID.")
			continue
		}

		_, resourceGroup, _, netappAccount, cPoolName, err = ParseCapacityPoolID(id)
		if err != nil {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid ID.")
			continue
		}

		cPoolFullName := CreateCapacityPoolFullName(resourceGroup, netappAccount, cPoolName)

		if rawProperties, ok = rawPoolMap["properties"].(map[string]interface{}); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned non-map properties.")
			continue
		}

		if qosType, ok = rawProperties["qosType"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid qosType.")
			continue
		}

		if poolID, ok = rawProperties["poolId"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid poolId.")
			continue
		}

		if serviceLevel, ok = rawProperties["serviceLevel"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid serviceLevel.")
			continue
		}

		if provisioningState, ok = rawProperties["provisioningState"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Capacity pool query returned invalid provisioningState.")
			continue
		}

		cpools = append(cpools,
			&CapacityPool{
				ID:                id,
				ResourceGroup:     resourceGroup,
				NetAppAccount:     netappAccount,
				Name:              cPoolName,
				FullName:          cPoolFullName,
				Location:          c.config.Location,
				Type:              "microsoft.netapp/netappaccounts/capacitypools",
				PoolID:            poolID,
				ServiceLevel:      serviceLevel,
				ProvisioningState: provisioningState,
				QosType:           qosType,
			})
	}

	return &cpools, nil
}

// discoverSubnetsWithRetry queries the Azure Resource Graph for all ANF-delegated subnets in the current location,
// retrying if the API request is throttled.
func (c Client) discoverSubnetsWithRetry(ctx context.Context) (subnets *[]*Subnet, err error) {
	discover := func() error {
		if subnets, err = c.discoverSubnets(ctx); err != nil && IsANFTooManyRequestsError(err) {
			return err
		}
		return backoff.Permanent(err)
	}

	notify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration.Truncate(10 * time.Millisecond),
		}).Debugf("Retrying subnets resource graph query.")
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = DefaultSDKTimeout
	expBackoff.MaxInterval = 5 * time.Second
	expBackoff.RandomizationFactor = 0.1
	expBackoff.InitialInterval = 5 * time.Second
	expBackoff.Multiplier = 1

	err = backoff.RetryNotify(discover, expBackoff, notify)

	return
}

// discoverSubnets queries the Azure Resource Graph for all ANF-delegated subnets in the current location.
func (c Client) discoverSubnets(ctx context.Context) (*[]*Subnet, error) {
	logFields := LogFields{
		"API": "GraphClient.Resources",
	}

	subscriptions := []string{c.config.SubscriptionID}
	query := fmt.Sprintf(`
    Resources
	| where type =~ 'Microsoft.Network/virtualNetworks' and location =~ '%s'
	| project subnets = (properties.subnets)
	| mv-expand subnets limit 2000
	| project subnetID = (subnets.id), delegations = (subnets.properties.delegations)
	| mv-expand delegations limit 2000
	| project subnetID, serviceName = (delegations.properties.serviceName)
	| where serviceName =~ 'Microsoft.NetApp/volumes'`, c.config.Location)
	resultFormat := resourcegraph.ResultFormat("objectArray")
	requestOptions := resourcegraph.QueryRequestOptions{ResultFormat: &resultFormat}

	request := resourcegraph.QueryRequest{
		Subscriptions: CreateStringPtrArray(subscriptions),
		Query:         &query,
		Options:       &requestOptions,
	}

	var rawResponse *http.Response
	responseCtx := policy.WithCaptureResponse(ctx, &rawResponse)

	response, err := c.sdkClient.GraphClient.Resources(responseCtx, request, nil)

	logFields["correlationID"] = GetCorrelationID(rawResponse)

	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Subnet query failed.")
		return nil, err
	}

	resourceList := response.QueryResponse

	Logc(ctx).WithFields(logFields).Debug("Read subnets from resource graph.")

	var subnets []*Subnet
	var data []interface{}
	var ok bool

	if resourceList.Data == nil || resourceList.Count == nil || *resourceList.Count == 0 {
		Logc(ctx).WithFields(logFields).Error("Subnet query returned no data.")
		return nil, errors.New("subnet query returned no data")
	} else if data, ok = resourceList.Data.([]interface{}); !ok {
		Logc(ctx).WithFields(logFields).Error("Subnet query returned invalid data.")
		return nil, errors.New("subnet pool query returned invalid data")
	}

	for _, rawSubnetInterface := range data {

		var rawSubnetMap map[string]interface{}
		var id, resourceGroup, virtualNetwork, subnet string

		if rawSubnetMap, ok = rawSubnetInterface.(map[string]interface{}); !ok {
			Logc(ctx).WithFields(logFields).Error("Subnet query returned non-map data.")
			continue
		}

		if id, ok = rawSubnetMap["subnetID"].(string); !ok {
			Logc(ctx).WithFields(logFields).Error("Subnet query returned non-string ID.")
			continue
		}

		_, resourceGroup, _, virtualNetwork, subnet, err = ParseSubnetID(id)
		if err != nil {
			Logc(ctx).WithFields(logFields).Error("Subnet query returned invalid ID.")
			continue
		}

		subnets = append(subnets,
			&Subnet{
				ID:             id,
				ResourceGroup:  resourceGroup,
				VirtualNetwork: virtualNetwork,
				Name:           subnet,
				FullName:       CreateSubnetFullName(resourceGroup, virtualNetwork, subnet),
				Location:       c.config.Location,
				Type:           "Microsoft.Network/virtualNetworks/subnets",
			})
	}

	return &subnets, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// API functions to match/search capacity pools
// ///////////////////////////////////////////////////////////////////////////////

// CapacityPools returns a list of all discovered ANF capacity pools.
func (c Client) CapacityPools() *[]*CapacityPool {
	var cPools []*CapacityPool

	for _, cPool := range c.sdkClient.AzureResources.CapacityPoolMap {
		cPools = append(cPools, cPool)
	}

	return &cPools
}

// capacityPool returns a single discovered capacity pool by its full name.
func (c Client) capacityPool(cPoolFullName string) *CapacityPool {
	return c.sdkClient.AzureResources.CapacityPoolMap[cPoolFullName]
}

// CapacityPoolsForStoragePools returns all discovered capacity pools matching all known storage pools,
// regardless of service levels or QOS types.
func (c Client) CapacityPoolsForStoragePools(ctx context.Context) []*CapacityPool {
	// This map deduplicates cPools from multiple storage pools
	cPoolMap := make(map[*CapacityPool]bool)

	// Build deduplicated map of cPools
	for _, sPool := range c.sdkClient.StoragePoolMap {
		for _, cPool := range c.CapacityPoolsForStoragePool(ctx, sPool, "", "") {
			cPoolMap[cPool] = true
		}
	}

	// Copy keys into a list of deduplicated cPools
	cPools := make([]*CapacityPool, 0)

	for cPool := range cPoolMap {
		cPools = append(cPools, cPool)
	}

	return cPools
}

// CapacityPoolsForStoragePool returns all discovered capacity pools matching the specified
// storage pool and service level.  The pools are shuffled to enable easier random selection.
func (c Client) CapacityPoolsForStoragePool(
	ctx context.Context, sPool storage.Pool, serviceLevel, qosType string,
) []*CapacityPool {
	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).WithField("storagePool", sPool.Name()).
		Tracef("Determining capacity pools for storage pool.")

	// This map tracks which capacity pools have passed the filters
	filteredCapacityPoolMap := make(map[string]bool)

	// Start with all capacity pools marked as passing the filters
	for cPoolFullName := range c.sdkClient.CapacityPoolMap {
		filteredCapacityPoolMap[cPoolFullName] = true
	}

	// If resource groups were specified, filter out non-matching capacity pools
	rgList := collection.SplitString(ctx, sPool.InternalAttributes()[resourceGroups], ",")
	if len(rgList) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !collection.ContainsString(rgList, cPool.ResourceGroup) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in resource groups [%s].",
					cPoolFullName, rgList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// If netapp accounts were specified, filter out non-matching capacity pools
	naList := collection.SplitString(ctx, sPool.InternalAttributes()[netappAccounts], ",")
	if len(naList) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			naName := cPool.NetAppAccount
			naFullName := CreateNetappAccountFullName(cPool.ResourceGroup, cPool.NetAppAccount)
			if !collection.ContainsString(naList, naName) && !collection.ContainsString(naList, naFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in netapp accounts [%s].",
					cPoolFullName, naList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// If capacity pools were specified, filter out non-matching capacity pools
	cpList := collection.SplitString(ctx, sPool.InternalAttributes()[capacityPools], ",")
	if len(cpList) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !collection.ContainsString(cpList, cPool.Name) && !collection.ContainsString(cpList, cPoolFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in capacity pools [%s].",
					cPoolFullName, cpList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// Filter out pools with non-matching service levels
	if serviceLevel != "" {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if cPool.ServiceLevel != serviceLevel {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not service level %s.",
					cPoolFullName, serviceLevel)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// Filter out pools with non-matching QOS type
	if qosType != "" {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if cPool.QosType != qosType {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not QOS type %s.",
					cPoolFullName, qosType)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// Build list of all capacity pools that have passed all filters
	cPools := make([]*CapacityPool, 0)
	for cPoolFullName, match := range filteredCapacityPoolMap {
		if match {
			cPools = append(cPools, c.sdkClient.CapacityPoolMap[cPoolFullName])
		}
	}

	// Shuffle the pools
	rand.Shuffle(len(cPools), func(i, j int) { cPools[i], cPools[j] = cPools[j], cPools[i] })

	return cPools
}

// EnsureVolumeInValidCapacityPool checks whether the specified volume exists in any capacity pool that is
// referenced by the backend config.  It returns nil if so, or if no capacity pools are named in the config.
func (c Client) EnsureVolumeInValidCapacityPool(ctx context.Context, volume *FileSystem) error {
	// Get a list of all capacity pools referenced in the config
	allCapacityPools := c.CapacityPoolsForStoragePools(ctx)

	// If we aren't restricting capacity pools, any capacity pool is OK
	if len(allCapacityPools) == 0 {
		return nil
	}

	// Always match by capacity pool full name
	cPoolFullName := CreateCapacityPoolFullName(volume.ResourceGroup, volume.NetAppAccount, volume.CapacityPool)

	for _, cPool := range allCapacityPools {
		if cPoolFullName == cPool.FullName {
			return nil
		}
	}

	return errors.NotFoundError("volume %s is part of another capacity pool not referenced "+
		"by this backend", volume.CreationToken)
}

// ///////////////////////////////////////////////////////////////////////////////
// API functions to match/search subnets
// ///////////////////////////////////////////////////////////////////////////////

// subnet returns a single subnet by its full name
func (c Client) subnet(subnetFullName string) *Subnet {
	return c.sdkClient.AzureResources.SubnetMap[subnetFullName]
}

// SubnetsForStoragePool returns all discovered subnets matching the specified storage pool.
func (c Client) SubnetsForStoragePool(ctx context.Context, sPool storage.Pool) []*Subnet {
	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).WithField("storagePool", sPool.Name()).Tracef("Determining subnets for storage pool.")

	// This map tracks which subnets have passed the filters
	filteredSubnetMap := make(map[string]bool)

	// Start with all subnets marked as passing the filters
	for subnetFullName := range c.sdkClient.SubnetMap {
		filteredSubnetMap[subnetFullName] = true
	}

	// If resource groups were specified, filter out non-matching subnets
	rgList := collection.SplitString(ctx, sPool.InternalAttributes()[resourceGroups], ",")
	if len(rgList) > 0 {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			if !collection.ContainsString(rgList, subnet.ResourceGroup) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring subnet %s, not in resource groups [%s].",
					subnetFullName, rgList)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	// If virtual network was specified, filter out non-matching subnets
	vn := sPool.InternalAttributes()[virtualNetwork]
	if vn != "" {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			vnName := subnet.VirtualNetwork
			vnFullName := CreateVirtualNetworkFullName(subnet.ResourceGroup, subnet.VirtualNetwork)
			if vn != vnName && vn != vnFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring subnet %s, not in virtual network %s.",
					subnetFullName, vn)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	// If subnet was specified, filter out non-matching capacity subnets
	sn := sPool.InternalAttributes()[subnet]
	if sn != "" {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			if sn != subnet.Name && sn != subnetFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring subnet %s, not equal to subnet %s.",
					subnetFullName, sn)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	// Build list of all subnets that have passed all filters
	filteredSubnetList := make([]*Subnet, 0)
	for subnetFullName, match := range filteredSubnetMap {
		if match {
			filteredSubnetList = append(filteredSubnetList, c.sdkClient.SubnetMap[subnetFullName])
		}
	}

	return filteredSubnetList
}

// RandomSubnetForStoragePool finds all discovered subnets matching the specified storage pool,
// and then returns one at random.
func (c Client) RandomSubnetForStoragePool(ctx context.Context, sPool storage.Pool) *Subnet {
	filteredSubnets := c.SubnetsForStoragePool(ctx, sPool)

	if len(filteredSubnets) == 0 {
		return nil
	}

	return filteredSubnets[crypto.RandomNumber(len(filteredSubnets))]
}

// AZ Client Functions for AKS Extension

func (c Client) FilteredCapacityPoolMap(
	ctx context.Context, rgFilter, naFilter, cpFilter []string,
) map[string]*CapacityPool {
	// This map tracks which capacity pools have passed the filters
	filteredCapacityPoolMap := make(map[string]bool)

	// Start with all capacity pools marked as passing the filters
	for cPoolFullName := range c.sdkClient.CapacityPoolMap {
		filteredCapacityPoolMap[cPoolFullName] = true
	}

	if len(rgFilter) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !collection.ContainsString(rgFilter, cPool.ResourceGroup) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Debugf("Ignoring capacity pool %s, not in resource groups [%s].", cPoolFullName, rgFilter)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	if len(naFilter) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			naName := cPool.NetAppAccount
			naFullName := CreateNetappAccountFullName(cPool.ResourceGroup, cPool.NetAppAccount)
			if !collection.ContainsString(naFilter, naName) && !collection.ContainsString(naFilter, naFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Debugf("Ignoring capacity pool %s, not in netapp accounts [%s].", cPoolFullName, naFilter)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	if len(cpFilter) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !collection.ContainsString(cpFilter, cPool.Name) && !collection.ContainsString(cpFilter, cPoolFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Debugf("Ignoring capacity pool %s, not in capacity pools [%s].", cPoolFullName, cpFilter)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	newCapacityPoolMap := make(map[string]*CapacityPool)
	for cpFullName, present := range filteredCapacityPoolMap {
		if present {
			newCapacityPoolMap[cpFullName] = c.sdkClient.CapacityPoolMap[cpFullName]
		}
	}

	return newCapacityPoolMap
}

func (c Client) FilteredSubnetMap(
	ctx context.Context, rgFilter []string, vnFilter, snFilter string,
) map[string]*Subnet {
	filteredSubnetMap := make(map[string]bool)

	for subnetFullName := range c.sdkClient.SubnetMap {
		filteredSubnetMap[subnetFullName] = true
	}

	if len(rgFilter) > 0 {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			if !collection.ContainsString(rgFilter, subnet.ResourceGroup) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Tracef("Ignoring subnet %s, not in resource groups [%s].", subnetFullName, rgFilter)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	if vnFilter != "" {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			vnName := subnet.VirtualNetwork
			vnFullName := CreateVirtualNetworkFullName(subnet.ResourceGroup, subnet.VirtualNetwork)
			if vnFilter != vnName && vnFilter != vnFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Debugf("Ignoring subnet %s, not in virtual network %s.", subnetFullName, vnFilter)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	if snFilter != "" {
		for subnetFullName, subnet := range c.sdkClient.SubnetMap {
			if snFilter != subnet.Name && snFilter != subnetFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).
					Debugf("Ignoring subnet %s, not equal to subnet %s.", subnetFullName, snFilter)
				filteredSubnetMap[subnetFullName] = false
			}
		}
	}

	newSubnetMap := make(map[string]*Subnet)
	for snFullName, present := range filteredSubnetMap {
		if present {
			newSubnetMap[snFullName] = c.sdkClient.SubnetMap[snFullName]
		}
	}

	return newSubnetMap
}

func (c Client) AvailabilityZones(ctx context.Context) ([]string, error) {
	logFields := LogFields{
		"API": "ResourceClient.QueryRegionInfo",
	}

	var availableZones []string
	regionInfoResponse, err := c.sdkClient.ResourceClient.QueryRegionInfo(ctx, c.config.Location, nil)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Failed to get availability zones.")
		return availableZones, err
	}

	for _, azMapping := range regionInfoResponse.RegionInfo.AvailabilityZoneMappings {
		if azMapping.IsAvailable != nil && *azMapping.IsAvailable && azMapping.AvailabilityZone != nil {
			availableZones = append(availableZones, *azMapping.AvailabilityZone)
		}
	}

	return availableZones, nil
}
