// Copyright 2021 NetApp, Inc. All Rights Reserved.

// Package sdk provides a high-level interface to the Azure NetApp Files SDK
package sdk

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/netapp/mgmt/2021-06-01/netapp"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

const (
	PServiceLevel  = "serviceLevel"
	PLocation      = "location"
	PSubnet        = "subnet"
	PCapacityPools = "capacityPools"

	// Reload Azure resources every n minutes
	refreshIntervalMinutes = 10

	// HTTP requests (such as create backend) time out at 90s
	discoveryTimeout = 80 * time.Second

	DoNotUseSPoolName = "Couldn't do a reverse lookup for storage pool name"
)

// AzureCapacityPoolCookie is a convenience bundle for mapping resources to the SDK API
type AzureCapacityPoolCookie struct {
	ResourceGroup    *string
	NetAppAccount    *string
	CapacityPoolName *string
	StoragePoolName  *string
}

// Subnet records details of a discovered Azure Subnet
type Subnet struct {
	Name           string
	VirtualNetwork *VirtualNetwork // backpointer to parent
}

// VirtualNetwork records details of a discovered Azure Virtual Network
type VirtualNetwork struct {
	Name          string
	Location      string
	Subnets       []Subnet
	ResourceGroup string
}

// NetAppAccount records details of a discovered ANF NetAppAccount
type NetAppAccount struct {
	Name          string
	Location      string
	CapacityPools []CapacityPool
}

// ResourceGroup records details of a discovered Azure ResourceGroup
type ResourceGroup struct {
	Name            string
	NetAppAccounts  []NetAppAccount
	VirtualNetworks []VirtualNetwork
}

// AzureResources is the toplevel cache for the set of things we discover about our Azure environment
type AzureResources struct {
	ResourceGroups []ResourceGroup
	StoragePoolMap map[string]storage.Pool
	m              *sync.Mutex
}

// ///////////////////////////////////////////////////////////////////////////////
// Cookie management routines
// This is the primary entry point for the rest of the SDK code to make
// queries of discovered resources.
// ///////////////////////////////////////////////////////////////////////////////

// createCookie generates a handle for accessing capacity pools from the require tuple values
func createCookie(rg string, naa string, cpool string, spool string) *AzureCapacityPoolCookie {

	cookie := &AzureCapacityPoolCookie{
		ResourceGroup:    &rg,
		NetAppAccount:    &naa,
		CapacityPoolName: &cpool,
		StoragePoolName:  &spool,
	}

	return cookie
}

// RegisterStoragePool makes a note of pools defined by the driver for later mapping
func (d *Client) registerStoragePool(spool storage.Pool) {
	d.SDKClient.AzureResources.StoragePoolMap[spool.Name()] = spool
}

// GetCookieByCapacityPoolName searches for a matching capacity pool name and returns an access cookie
func (d *Client) GetCookieByCapacityPoolName(poolname string) (*AzureCapacityPoolCookie, error) {

	// Don't allow queries during a rebuild
	d.SDKClient.AzureResources.m.Lock()
	defer d.SDKClient.AzureResources.m.Unlock()

	cpool, err := d.getCapacityPool(poolname)
	if err != nil {
		return nil, err
	}

	// a rg:naa:cp tuple could belong to multiple spools? Pick one? Does it matter?
	//
	// The only current user of this value is CreateClone()->CreateVolume(); working around
	// this issue there by inheriting the CapacityPool from the sourcevol and passing it
	// in via the FilesystemCreateRequest.
	return createCookie(cpool.ResourceGroup, cpool.NetAppAccount, cpool.Name, DoNotUseSPoolName), nil
}

// GetCookieByStoragePoolName searches for a cookie with a matching storage pool name
func (d *Client) GetCookieByStoragePoolName(
	ctx context.Context, spoolname, serviceLevel string,
) (*AzureCapacityPoolCookie, error) {
	spool := d.SDKClient.AzureResources.StoragePoolMap[spoolname]
	if spool == nil {
		return nil, fmt.Errorf("no pool '%s' registered", spoolname)
	}

	// Don't allow queries during a rebuild
	d.SDKClient.AzureResources.m.Lock()
	defer d.SDKClient.AzureResources.m.Unlock()

	cpool, err := d.randomCapacityPoolWithStoragePoolAttributes(ctx, spool, serviceLevel)

	if err != nil {
		return nil, err
	}

	if cpool == nil {
		return nil, errors.New("no capacity pools found")
	}

	return createCookie(cpool.ResourceGroup, cpool.NetAppAccount, cpool.Name, spoolname), nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Internal functions to do discovery
// ///////////////////////////////////////////////////////////////////////////////

func (d *Client) discoverResourceGroups(ctx context.Context) (*[]string, error) {

	var rgs []string

	// Get a temporary GroupsClient, we don't need this anywhere else.
	gc := resources.NewGroupsClient(d.config.SubscriptionID)
	gc.Authorizer, _ = d.SDKClient.AuthConfig.Authorizer()

	list, err := gc.ListComplete(ctx, "", nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching resource groups: %v", err)
	}

	for ; list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating resource groups: %v", err)
		}

		rg := utils.DerefString(list.Value().Name)
		rgs = append(rgs, rg)
	}

	return &rgs, nil
}

// Get a list of CapacityPools within a given Resource Group : NetAppAccount pair
func (d *Client) discoverCapacityPools(ctx context.Context, rgroup string, naa string) (*[]CapacityPool, error) {

	var cpools []CapacityPool

	list, err := d.SDKClient.PoolsClient.ListComplete(ctx, rgroup, naa)
	if err != nil {
		Logc(ctx).Errorf("error fetching capacity pools for rg %s, account %s: %v\n", rgroup, naa, err)
		return nil, err
	}

	for ; list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating capacity pools for rg %s, account %s: %v", rgroup, naa, err)
		}

		p := list.Value()
		poolName := utils.DerefString(p.Name)

		if exists, _ := d.capacityPoolWithName(poolName); exists != nil {
			Logc(ctx).Errorf(
				"Ignoring discovered capacity pool '%s' in resource group '%s' because its name is not unique",
				exists.Name, rgroup)
			continue
		}

		if p.QosType == netapp.QosTypeManual {
			Logc(ctx).Warningf(
				"Ignoring discovered capacity pool '%s' in resource group '%s' because it uses manual QoS",
				poolName, rgroup)
			continue
		}

		cpools = append(cpools,
			CapacityPool{
				Location:          utils.DerefString(p.Location),
				ID:                utils.DerefString(p.ID),
				Fullname:          poolName,
				Name:              poolShortname(poolName),
				ResourceGroup:     rgroup,
				NetAppAccount:     naa,
				Type:              utils.DerefString(p.Type),
				Tags:              p.Tags,
				PoolID:            utils.DerefString(p.PoolID),
				ServiceLevel:      string(p.ServiceLevel),
				ProvisioningState: utils.DerefString(p.PoolProperties.ProvisioningState),
				QosType:           string(p.QosType),
			})
	}

	return &cpools, nil
}

// Get a list of NetAppAccounts within a given Resource Group
func (d *Client) discoverNetAppAccounts(ctx context.Context, rgroup string) (*[]NetAppAccount, error) {

	var netappAccounts []NetAppAccount

	list, err := d.SDKClient.AccountsClient.ListComplete(ctx, rgroup)
	if err != nil {
		return nil, fmt.Errorf("error fetching netappaccounts for %s: %v", rgroup, err)
	}

	for ; list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating netappaccounts for rg %s: %v", rgroup, err)
		}

		naa := list.Value()

		na := NetAppAccount{
			Name:     utils.DerefString(naa.Name),
			Location: utils.DerefString(naa.Location),
		}

		// Go ahead and get the capacity pools for this rg:naa pair
		cpools, err := d.discoverCapacityPools(ctx, rgroup, na.Name)
		if err != nil {
			return nil, err
		}
		na.CapacityPools = *cpools

		netappAccounts = append(netappAccounts, na)
	}

	return &netappAccounts, nil
}

// Get a list of Subnets within a given Resource_Group:VirtualNetwork pairing
func (d *Client) discoverSubnets(ctx context.Context, rgroup string, vn VirtualNetwork) (*[]Subnet, error) {

	var subnets []Subnet

	list, err := d.SDKClient.SubnetsClient.ListComplete(ctx, rgroup, vn.Name)
	if err != nil {
		return nil, fmt.Errorf("error fetching subnets for rg %s, vn %s: %v", rgroup, vn.Name, err)
	}

	for ; list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating subnets for rg %s, vn %s: %v", rgroup, vn.Name, err)
		}

		// Check the subnet delegations; only add this subnet to the list if it has
		// a proper ANF delegation.  Note that even Microsoft can't decide if we are
		// spelled "NetApp" or "Netapp" - man, did that take a while to debug.
		// Update: item.Name can be a strange uuid-looking string as well as the more
		// readable "NetAppDelegation", so just ignore it and check the value.
		delegations := *list.Value().Delegations
		for _, item := range delegations {
			serviceName := utils.DerefString(item.ServiceName)
			if serviceName == "Microsoft.NetApp/volumes" || serviceName == "Microsoft.Netapp/volumes" {
				sn := Subnet{
					Name:           utils.DerefString(list.Value().Name),
					VirtualNetwork: &vn,
				}
				subnets = append(subnets, sn)
			}
		}
	}

	return &subnets, nil
}

// Get a list of VirtualNetworks within a given Resource Group
func (d *Client) discoverVirtualNetworks(ctx context.Context, rgroup string) (*[]VirtualNetwork, error) {

	var vnets []VirtualNetwork

	list, err := d.SDKClient.VirtualNetworksClient.ListComplete(ctx, rgroup)
	if err != nil {
		return nil, fmt.Errorf("error fetching virtual networks for resource group %s: %v", rgroup, err)
	}

	for ; list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating virtual networks for resource group %s: %v", rgroup, err)
		}
		vn := VirtualNetwork{
			Name:          utils.DerefString(list.Value().Name),
			Location:      utils.DerefString(list.Value().Location),
			ResourceGroup: rgroup,
		}

		if otherRG, _ := d.resourceGroupForVirtualNetwork(vn.Name); otherRG != nil && *otherRG != rgroup {
			Logc(ctx).Errorf(
				"duplicate virtual network '%s' in resource group '%s' ignored during discovery; unique names required",
				vn.Name, *otherRG)
			continue
		}

		// Populate subnets for this virtual network
		subnets, err := d.discoverSubnets(ctx, rgroup, vn)
		if err != nil {
			return nil, err
		}

		// Don't populate the vnet if it has no ANF subnets
		if len(*subnets) > 0 {
			vn.Subnets = *subnets
			vnets = append(vnets, vn)
		}
	}

	return &vnets, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Top level init functions
// ///////////////////////////////////////////////////////////////////////////////

func (d *Client) DiscoverAzureResources(ctx context.Context) (returnError error) {

	// Start from scratch each time we are called.  All discovered resources are
	// nested under ResourceGroups.
	newResourceGroups := make([]ResourceGroup, 0)

	defer func() {
		if returnError != nil {
			Logc(ctx).WithField("error", returnError).Debug("Discovery error, not retaining any discovered resources.")
			return
		}

		// Lock mutex while swapping out the discovered resources
		d.SDKClient.AzureResources.m.Lock()
		defer d.SDKClient.AzureResources.m.Unlock()

		d.SDKClient.AzureResources.ResourceGroups = newResourceGroups
		Logc(ctx).Debug("Switched to newly discovered resources.")
	}()

	// Get a list of resource group names and populate the cache
	groups, returnError := d.discoverResourceGroups(ctx)
	if returnError != nil {
		return returnError
	}

	for _, g := range *groups {
		rg := ResourceGroup{Name: g}

		// Fetch NetAppAccounts for this RG
		naas, returnError := d.discoverNetAppAccounts(ctx, g)
		if returnError != nil {
			return returnError
		}

		// Fetch subnets for this RG
		vnets, returnError := d.discoverVirtualNetworks(ctx, g)
		if returnError != nil {
			return returnError
		}

		// Don't track resource groups that don't have either an ANF accounts or ANF subnets
		if len(*naas) > 0 || len(*vnets) > 0 {
			rg.NetAppAccounts = *naas
			rg.VirtualNetworks = *vnets
			newResourceGroups = append(newResourceGroups, rg)
		} else {
			Logc(ctx).Debugf("Ignoring discovered resource group '%s' because it has no ANF accounts or subnets.\n", g)
		}
	}

	// Detect the lack of any resources: can occur when no connectivity, etc.
	// Would like a better way of proactively finding out there is something wrong
	// at a very basic level.  (Reproduce this by turning off your network!)
	numResourceGroups, numCapacityPools, numVnets := d.countAzureResources(newResourceGroups)
	if numResourceGroups == 0 {
		returnError = errors.New("no resource groups discovered; check connectivity, credentials")
		return
	}
	if numCapacityPools == 0 {
		returnError = errors.New("no capacity pools discovered; volume provisioning may fail until corrected")
		return
	}
	if numVnets == 0 {
		returnError = errors.New("no virtual networks discovered; volume provisioning may fail until corrected")
		return
	}

	Logc(ctx).WithFields(log.Fields{
		"resourceGroups":  numResourceGroups,
		"capacityPools":   numCapacityPools,
		"virtualNetworks": numVnets,
	}).Info("Discovered Azure resources.")

	return
}

func (d *Client) dumpAzureResources(ctx context.Context) {

	Logc(ctx).Debugf("Discovered Azure Resources:\n")
	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		Logc(ctx).Debugf("  Resource Group: %s\n", rg.Name)
		for _, na := range rg.NetAppAccounts {
			Logc(ctx).Debugf("    ANF Account: %s, Location: %s\n", na.Name, na.Location)
			for _, cp := range na.CapacityPools {
				Logc(ctx).Debugf("      CPool: %s, [%s, %s]\n", cp.Name, cp.ServiceLevel, cp.QosType)
			}
		}
		for _, vn := range rg.VirtualNetworks {
			for _, sn := range vn.Subnets {
				Logc(ctx).Debugf("    Subnet: %s, Location: %s (vnet: %s)\n", sn.Name, vn.Location, vn.Name)
			}
		}
	}
}

// countAzureResources accepts a list of resource groups and returns the number of encompassed
// resource groups, capacity pools, and subnets.
func (d *Client) countAzureResources(resourceGroups []ResourceGroup) (int, int, int) {

	vnetCount := 0
	cpoolCount := 0

	for _, rg := range resourceGroups {
		for _, na := range rg.NetAppAccounts {
			cpoolCount += len(na.CapacityPools)
		}
		for _, vn := range rg.VirtualNetworks {
			vnetCount += len(vn.Subnets)
		}
	}

	return len(resourceGroups), cpoolCount, vnetCount
}

// refreshAzureResources wraps the toplevel discovery process for the timer thread
func (d *Client) refreshAzureResources(ctx context.Context) error {

	// (re-)Discover what we have to work with in Azure
	Logc(ctx).Debugf("Discovering Azure resources")

	var discoveryErrors []string

	err := d.DiscoverAzureResources(ctx)
	if err != nil {
		Logc(ctx).Errorf("error discovering resources: %v", err)
	}

	// Ensure we have harmony between the discovered Capacity Pools and
	// the Capacity Pools (if any) specified in the backend config pool
	if allCpools := d.GetCapacityPoolNames(); len(allCpools) == 0 {
		Logc(ctx).Errorf("No capacity pool discovered.")
	} else {
		for poolName, pool := range d.SDKClient.AzureResources.StoragePoolMap {

			var validCpools []string
			realCpoolNamesFilter := d.filterRealCapacityPools(ctx, pool)

			if len(realCpoolNamesFilter) == 0 {
				msg := fmt.Sprintf("No valid capacity pool found for the pool %s", poolName)
				Logc(ctx).Errorf(msg)
				discoveryErrors = append(discoveryErrors, msg)
				continue
			}

			// This condition ensures that the capacity pool that does exist also matches pool's
			// attributes such as location, service level and subnet
			cpoolsWithAttr, filterErr := d.filterCpoolBasedOnAttributesAndCpoolNames(
				ctx, pool.InternalAttributes()[PLocation], pool.InternalAttributes()[PServiceLevel],
				pool.InternalAttributes()[PSubnet], poolName, realCpoolNamesFilter)
			if filterErr != nil {
				msg := fmt.Sprintf("Unable to get list of capacity pools based on location, "+
					"service level, subnet and capacity pool name filters for the pool %v: %v", poolName, filterErr)
				Logc(ctx).Errorf(msg)
				discoveryErrors = append(discoveryErrors, msg)
			} else if cpoolsWithAttr == nil || len(*cpoolsWithAttr) == 0 {
				msg := fmt.Sprintf("No capacity pools found based on location, "+
					"service level, subnet and capacity pool name filters for the pool %v", poolName)
				Logc(ctx).Errorf(msg)
				discoveryErrors = append(discoveryErrors, msg)
			} else if len(*cpoolsWithAttr) > 0 {
				validCpools = d.ExtractCapacityPoolNames(cpoolsWithAttr)
			}

			// Print the mapping in the logs so that it is easy to understand the mapping at the time of the operation.
			Logc(ctx).Debugf("Storage Pool '%v' mapped to capacity pool '%v'.", poolName,
				strings.Join(validCpools, ", "))
		}
	}

	// This is noisy, hide it behind api tracing.
	if d.config.DebugTraceFlags["api"] {
		d.dumpAzureResources(ctx)
	}

	var discoveryErrorMsg string
	if len(discoveryErrors) > 0 {
		discoveryErrorMsg = strings.Join(discoveryErrors, "; ")
	}

	if err != nil {
		if discoveryErrorMsg != "" {
			return fmt.Errorf("%v; Error(s) after resource discovery: %s", err, discoveryErrorMsg)
		}
	} else if discoveryErrorMsg != "" {
		return fmt.Errorf("error(s) after resource discovery: %s", discoveryErrorMsg)
	}

	return err
}

// refreshTimer waits refreshIntervalMinutes, does the refresh work, and then reschedules itself
func (d *Client) refreshTimer(ctx context.Context) {

	nextRefresh := time.Now().Add(time.Minute * refreshIntervalMinutes)

	Logc(ctx).WithFields(log.Fields{
		"time": nextRefresh,
	}).Debugf("Resource refresh in %d minutes", refreshIntervalMinutes)

	time.Sleep(time.Until(nextRefresh))

	// Exit if the backend has been terminated
	if d.terminated {
		return
	}

	_ = d.refreshAzureResources(ctx)

	go d.refreshTimer(ctx)
}

// discoveryInit initializes the discovery pieces at startup
func (d *Client) discoveryInit(ctx context.Context) error {

	// Discover resources at startup synchronously, then kick off the refresh timer thread
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.refreshAzureResources(ctx)
	}()
	select {
	case <-time.After(discoveryTimeout):
		return errors.New("timeout discovering resources")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	go d.refreshTimer(ctx)
	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// internal API functions to match/search cached values
// ///////////////////////////////////////////////////////////////////////////////

// getNetAppAccountsForResourceGroup returns a list of all NetAppAccounts in a Resource Group
func (d *Client) getNetAppAccountsForResourceGroup(rgroup string) *[]NetAppAccount {
	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		if rg.Name == rgroup {
			return &rg.NetAppAccounts
		}
	}
	return nil
}

// getCapacityPoolsForNetAppAccount returns a list of all capacity pools in a given resource group : netapp account pair
func (d *Client) getCapacityPoolsForNetAppAccount(rgroup string, naa string) *[]CapacityPool {
	naas := d.getNetAppAccountsForResourceGroup(rgroup)
	for _, na := range *naas {
		if na.Name == naa {
			return &na.CapacityPools
		}
	}
	return nil
}

// getCapacityPoolsForResourceGroup returns a list of all capacity pools in a given resource group
func (d *Client) getCapacityPoolsForResourceGroup(rgroup string) *[]CapacityPool {

	var cpools []CapacityPool

	naas := d.getNetAppAccountsForResourceGroup(rgroup)
	for _, na := range *naas {
		cpools = append(cpools, na.CapacityPools...)
	}

	return &cpools
}

// getCapacityPools returns a list of _ALL_ ANF Capacity Pools
func (d *Client) getCapacityPools() *[]CapacityPool {

	var cpools []CapacityPool

	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		rgcps := d.getCapacityPoolsForResourceGroup(rg.Name)
		cpools = append(cpools, *rgcps...)
	}

	return &cpools
}

// GetCapacityPoolNames returns names of _ALL_ ANF Capacity Pools
func (d *Client) GetCapacityPoolNames() []string {
	return d.ExtractCapacityPoolNames(d.getCapacityPools())
}

// ExtractCapacityPoolNames returns names of Capacity Pools
func (d *Client) ExtractCapacityPoolNames(allpools *[]CapacityPool) []string {

	var cpoolNames []string

	if allpools != nil {
		for _, p := range *allpools {
			cpoolNames = append(cpoolNames, poolShortname(p.Name))
		}
	}

	return cpoolNames
}

// getCapacityPool returns a single capacity pool by name
func (d *Client) getCapacityPool(poolname string) (*CapacityPool, error) {

	allpools := d.getCapacityPools()

	for _, p := range *allpools {
		if poolShortname(p.Name) == poolname {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("couldn't find pool %v", poolname)
}

// getSubnetsForResourceGroup returns a list of all subnets in a given resource group
func (d *Client) getSubnetsForResourceGroup(rgroup string) *[]Subnet {

	var subnets []Subnet

	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		if rg.Name == rgroup {
			for _, vnet := range rg.VirtualNetworks {
				subnets = append(subnets, vnet.Subnets...)
			}
		}
	}

	return &subnets
}

// getSubnetsForLocation returns a list of subnets in a given location
func (d *Client) getSubnetsForLocation(location string) *[]Subnet {

	var subnets []Subnet

	allsubnets := d.getSubnets()

	for _, sn := range *allsubnets {
		if sn.VirtualNetwork.Location == location {
			subnets = append(subnets, sn)
		}
	}

	return &subnets
}

// getSubnetsForLocationAndVNet returns a list of subnets in a given location restricted by vnet
func (d *Client) getSubnetsForLocationAndVNet(vnet string, location string) *[]Subnet {
	var selection []Subnet

	allsubnets := d.getSubnetsForLocation(location)

	for _, sn := range *allsubnets {
		if sn.VirtualNetwork.Name == vnet {
			selection = append(selection, sn)
		}
	}

	return &selection
}

// randomSubnetForLocation returns a random subnet in a given location
func (d *Client) randomSubnetForLocation(optionalVnet string, location string) *Subnet {
	var subnet *Subnet
	var subnets *[]Subnet

	if optionalVnet == "" {
		subnets = d.getSubnetsForLocation(location)
	} else {
		subnets = d.getSubnetsForLocationAndVNet(optionalVnet, location)
	}
	if subnets != nil && len(*subnets) > 0 {
		subnet = &(*subnets)[0]
		if len(*subnets) > 0 {
			rnd := 0
			max := len(*subnets)
			if max > 1 {
				rnd = rand.Intn(max)
			}
			subnet = &(*subnets)[rnd]
		}
	}

	return subnet
}

// getSubnets returns a list of _ALL_ subnets
func (d *Client) getSubnets() *[]Subnet {

	var subnets []Subnet

	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		for _, vnet := range rg.VirtualNetworks {
			subnets = append(subnets, vnet.Subnets...)
		}
	}

	return &subnets
}

// getSubnet returns a subnet for a given subnet name : location
func (d *Client) getSubnet(name string, location string) *Subnet {
	allsubnets := d.getSubnets()

	for _, sn := range *allsubnets {
		if sn.Name == name && sn.VirtualNetwork.Location == location {
			return &sn
		}
	}

	return nil
}

// getSubnet returns a VirtualNetwork for a given subnet & location
func (d *Client) virtualNetworkForSubnet(subnet string, location string) (*VirtualNetwork, error) {
	sn := d.getSubnet(subnet, location)
	if sn != nil {
		return sn.VirtualNetwork, nil
	}

	return nil, fmt.Errorf("could not find subnet %s in %s", subnet, location)
}

// resourceGroupForVirtualNetwork finds a resource group name for a virtual network name.
func (d *Client) resourceGroupForVirtualNetwork(vNet string) (*string, error) {
	for _, rg := range d.SDKClient.AzureResources.ResourceGroups {
		for _, vn := range rg.VirtualNetworks {
			if vn.Name == vNet {
				return &rg.Name, nil
			}
		}
	}
	return nil, fmt.Errorf("no resource group found for virtual network %s", vNet)
}

// capacityPoolWithName returns a capacity pool with a given name (aka, "capacityPoolExists")
func (d *Client) capacityPoolWithName(name string) (*CapacityPool, error) {
	allpools := d.getCapacityPools()
	for _, cp := range *allpools {
		if cp.Name == name {
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("no pool '%s' found", name)
}

// capacityPoolsWithLocation returns all capacity pools that are in location x
func (d *Client) capacityPoolsWithLocation(location string) (*[]CapacityPool, error) {
	var cpools []CapacityPool

	allpools := d.getCapacityPools()
	for _, cp := range *allpools {
		if cp.Location == location {
			cpools = append(cpools, cp)
		}
	}

	return &cpools, nil
}

// capacityPoolsWithServiceLevel returns all capacity pools that have Service Level x
func (d *Client) capacityPoolsWithServiceLevel(level string) (*[]CapacityPool, error) {
	var cpools []CapacityPool

	allpools := d.getCapacityPools()
	for _, cp := range *allpools {
		if cp.ServiceLevel == level {
			cpools = append(cpools, cp)
		}
	}

	return &cpools, nil
}

// HasCapacityPoolForServiceLevel returns true if there is one or more capacity pool with the specified service level.
func (d *Client) HasCapacityPoolForServiceLevel(level string) bool {

	pools, _ := d.capacityPoolsWithServiceLevel(level)
	return len(*pools) > 0
}

// capacityPoolsWithSubnet returns all capacity pools that .. okay, capacity pools don't
// have subnets, but they do share locations with subnets.  This doesn't really make a lot
// of sense, but we want to be able to specify subnets, so for now, return any capacity
// pool that shares the location of the specified subnet.
func (d *Client) capacityPoolsWithSubnet(subnet string) (*[]CapacityPool, error) {
	var location string
	var cpools []CapacityPool

	subnets := d.getSubnets()
	allpools := d.getCapacityPools()

	// Get location for specified subnet
	for _, sn := range *subnets {
		if sn.Name == subnet {
			location = sn.VirtualNetwork.Location
		}
	}

	if location == "" {
		return nil, fmt.Errorf("couldn't find subnet '%s'", subnet)
	}

	for _, cp := range *allpools {
		if cp.Location == location {
			cpools = append(cpools, cp)
		}
	}

	return &cpools, nil
}

func (d *Client) commonSet(c1 *[]CapacityPool, c2 *[]CapacityPool) *[]CapacityPool {
	var common []CapacityPool

	for _, c1elem := range *c1 {
		for _, c2elem := range *c2 {
			if c1elem.Name == c2elem.Name {
				common = append(common, c1elem)
			}
		}
	}

	return &common
}

// filterRealCapacityPools verifies the names of the capacity pool name filters (if set) in the storage pool are
// still valid or not, and returns list of the valid capacity pool name filters.
// If storage pool has no capacity pool specified then all the real capacity pools names are returned
func (d *Client) filterRealCapacityPools(ctx context.Context, pool storage.Pool) []string {
	var realCpoolNamesFilter []string

	allCpools := d.GetCapacityPoolNames()
	if len(allCpools) == 0 {
		Logc(ctx).Errorf("There are no capacity pools.")
		return realCpoolNamesFilter
	}

	// This condition lets us know if a capacity pool name that is mentioned
	// in the storage pool exists or not
	cpoolList := utils.SplitString(ctx, pool.InternalAttributes()[PCapacityPools], ",")
	if len(cpoolList) != 0 {
		for _, capacityPool := range cpoolList {
			if !utils.SliceContainsString(allCpools, capacityPool) {
				Logc(ctx).Errorf("Invalid value for capacity pool %s in pool %s", capacityPool, pool.Name())
			} else {
				realCpoolNamesFilter = append(realCpoolNamesFilter, capacityPool)
			}
		}
	} else {
		realCpoolNamesFilter = allCpools
	}

	return realCpoolNamesFilter
}

// filterCpoolBasedOnAttributes returns all capacity pools that match specified attributes
func (d *Client) filterCpoolBasedOnAttributes(location, servicelevel, subnet string) (*[]CapacityPool, error) {

	var withLocs *[]CapacityPool
	var withLevs *[]CapacityPool
	var withNets *[]CapacityPool

	// We don't always actually specify any attributes.  In that case, return everything.
	if location == "" && servicelevel == "" && subnet == "" {
		return d.getCapacityPools(), nil
	}

	if location != "" {
		withLocs, _ = d.capacityPoolsWithLocation(location)
	}
	if servicelevel != "" {
		withLevs, _ = d.capacityPoolsWithServiceLevel(servicelevel)
	}
	if subnet != "" {
		withNets, _ = d.capacityPoolsWithSubnet(subnet)
	}

	var common *[]CapacityPool
	haveLocBasedCpools := withLocs != nil && len(*withLocs) > 0
	haveServiceLevelBasedCpools := withLevs != nil && len(*withLevs) > 0
	haveSubnetBasedCpools := withNets != nil && len(*withNets) > 0

	// If Failed to discover cpool against any of the attributes then return an empty list
	if (location != "" && !haveLocBasedCpools) || (servicelevel != "" && !haveServiceLevelBasedCpools) ||
		(subnet != "" && !haveSubnetBasedCpools) {
		return common, nil
	}

	cpoolFilterLists := []*[]CapacityPool{withLocs, withLevs, withNets}

	for _, filteredList := range cpoolFilterLists {
		if filteredList != nil && len(*filteredList) > 0 {
			if common == nil || len(*common) == 0 {
				common = filteredList
			} else {
				common = d.commonSet(common, filteredList)
			}
		}
	}

	return common, nil
}

// filterCpoolBasedOnAttributesAndCpoolNames returns all capacity pools that match specified attributes and cpool names
func (d *Client) filterCpoolBasedOnAttributesAndCpoolNames(
	ctx context.Context, location, servicelevel, subnet, poolName string, cpoolNamesFilter []string,
) (*[]CapacityPool, error) {

	cpools, err := d.filterCpoolBasedOnAttributes(location, servicelevel, subnet)
	if err != nil {
		return nil, err
	}

	// Filter the list of capacity pools based on the capacity pool names when supplied
	if len(cpoolNamesFilter) > 0 && cpools != nil && len(*cpools) > 0 {
		var filteredCpools []CapacityPool
		for _, cp := range *cpools {
			if utils.SliceContainsString(cpoolNamesFilter, poolShortname(cp.Name)) {
				filteredCpools = append(filteredCpools, cp)
			} else {
				Logc(ctx).Debugf("Skipping capacity pool '%s' as it is not part of the listed capacity pools '%v"+
					"' for the storage pool '%v'", poolShortname(cp.Name), strings.Join(cpoolNamesFilter, ","),
					poolName)
			}
		}

		return &filteredCpools, nil
	}

	return cpools, nil
}

// randomCapacityPoolWithStoragePoolAttributes searches for a capacity pool that matches any
// passed-in attributes
func (d *Client) randomCapacityPoolWithStoragePoolAttributes(
	ctx context.Context, spool storage.Pool, serviceLevel string,
) (*CapacityPool, error) {

	var cp *CapacityPool

	// Ensure that names in the capacity Pool filter are still valid
	realCpoolNamesFilter := d.filterRealCapacityPools(ctx, spool)

	if len(realCpoolNamesFilter) == 0 {
		Logc(ctx).Errorf("No valid capacity pool name found in the storage pool %s", spool.Name())
		return cp, nil
	}

	cpools, err := d.filterCpoolBasedOnAttributesAndCpoolNames(
		ctx, spool.InternalAttributes()[PLocation], serviceLevel, spool.InternalAttributes()[PSubnet], spool.Name(),
		utils.SplitString(ctx, spool.InternalAttributes()[PCapacityPools], ","),
	)
	if err != nil {
		return nil, err
	}

	if cpools != nil && len(*cpools) > 0 {

		cpoolNames := d.ExtractCapacityPoolNames(cpools)
		Logc(ctx).Debugf("Capacity Pool(s) '%s' discovered for the storage pool '%s'.", strings.Join(cpoolNames, ","),
			spool.Name())

		rnd := 0
		max := len(*cpools)
		if max > 1 {
			rnd = rand.Intn(max)
		}
		cp = &(*cpools)[rnd]

		Logc(ctx).Debugf("Capacity Pool '%s' selected.", poolShortname(cp.Name))
	}

	return cp, nil
}
