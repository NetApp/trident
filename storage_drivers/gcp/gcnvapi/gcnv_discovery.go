// Copyright 2024 NetApp, Inc. All Rights Reserved.

// Package gcnvapi provides a high-level interface to the Google Cloud NetApp Volumes SDK
package gcnvapi

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"cloud.google.com/go/netapp/apiv1/netapppb"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"
	"google.golang.org/api/iterator"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const (
	serviceLevel       = "serviceLevel"
	network            = "network"
	capacityPools      = "capacityPools"
	DefaultMaxCacheAge = 10 * time.Minute
)

// ///////////////////////////////////////////////////////////////////////////////
// Top level discovery functions
// ///////////////////////////////////////////////////////////////////////////////

// RefreshGCNVResources refreshes the cache of discovered GCNV resources and validates
// them against our known storage pools.
func (c Client) RefreshGCNVResources(ctx context.Context) error {
	// Check if it is time to update the cache
	if time.Now().Before(c.sdkClient.GCNVResources.lastUpdateTime.Add(c.config.MaxCacheAge)) {
		Logc(ctx).Debugf("Cached resources not yet %v old, skipping refresh.", c.config.MaxCacheAge)
		return nil
	}

	// (re-)Discover what we have to work with in GCNV
	Logc(ctx).Debugf("Discovering GCNV resources.")
	discoveryErr := multierr.Combine(c.DiscoverGCNVResources(ctx))

	// This is noisy, hide it behind api tracing.
	c.dumpGCNVResources(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"])

	// Warn about anything in the config that doesn't match any discovered resources
	c.checkForNonexistentCapacityPools(ctx)
	c.checkForNonexistentNetworks(ctx)

	// Return errors for any storage pool that cannot be satisfied by discovered resources
	poolErrors := multierr.Combine(c.checkForUnsatisfiedPools(ctx)...)
	discoveryErr = multierr.Combine(discoveryErr, poolErrors)

	return discoveryErr
}

// DiscoverGCNVResources rediscovers the GCNV resources we care about and updates the cache.
func (c Client) DiscoverGCNVResources(ctx context.Context) (returnError error) {
	// Start from scratch each time we are called.
	newCapacityPoolMap := make(map[string]*CapacityPool)

	defer func() {
		if returnError != nil {
			Logc(ctx).WithError(returnError).Debug("Discovery error, not retaining any discovered resources.")
			return
		}

		// Swap the newly discovered resources into the cache only if discovery succeeded.
		c.sdkClient.GCNVResources.CapacityPoolMap = newCapacityPoolMap
		c.sdkClient.GCNVResources.lastUpdateTime = time.Now()

		Logc(ctx).Debug("Switched to newly discovered resources.")
	}()

	// Discover capacity pools
	cPools, returnError := c.discoverCapacityPoolsWithRetry(ctx)
	if returnError != nil {
		return
	}

	// Update maps with all data from discovered capacity pools
	for _, cPool := range *cPools {
		newCapacityPoolMap[cPool.FullName] = cPool
	}

	// Detect the lack of any resources: can occur when no connectivity, etc.
	// Would like a better way of proactively finding out there is something wrong
	// at a very basic level.  (Reproduce this by turning off your network!)
	numCapacityPools := len(newCapacityPoolMap)

	if numCapacityPools == 0 {
		return errors.New("no GCNV storage pools discovered; volume provisioning may fail until corrected")
	}

	Logc(ctx).WithFields(LogFields{
		"capacityPools": numCapacityPools,
	}).Info("Discovered GCNV resources.")

	return
}

// dumpGCNVResources writes a hierarchical representation of discovered resources to the log.
func (c Client) dumpGCNVResources(ctx context.Context, driverName string, discoveryTraceEnabled bool) {
	Logd(ctx, driverName, discoveryTraceEnabled).Tracef("Discovered GCNV Resources:")

	for _, cp := range c.sdkClient.GCNVResources.CapacityPoolMap {
		Logd(ctx, driverName, discoveryTraceEnabled).Tracef("CPool: %s, [%s, %s]",
			cp.Name, cp.ServiceLevel, cp.NetworkName)
	}
}

// checkForUnsatisfiedPools returns one or more errors if one or more configured storage pools
// are satisfied by no capacity pools.
func (c Client) checkForUnsatisfiedPools(ctx context.Context) (discoveryErrors []error) {
	// Ensure every storage pool matches one or more capacity pools
	for sPoolName, sPool := range c.sdkClient.GCNVResources.StoragePoolMap {

		// Find all capacity pools that work for this storage pool
		cPools := c.CapacityPoolsForStoragePool(ctx, sPool, sPool.InternalAttributes()[serviceLevel])

		if len(cPools) == 0 {

			err := fmt.Errorf("no GCNV storage pools found for Trident pool %s", sPoolName)
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

// checkForNonexistentCapacityPools logs warnings if any configured capacity pools do not
// match discovered capacity pools in the resource cache.
func (c Client) checkForNonexistentCapacityPools(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.GCNVResources.StoragePoolMap {

		// Build list of capacity pool names
		cpNames := make([]string, 0)
		for _, cacheCP := range c.sdkClient.GCNVResources.CapacityPoolMap {
			cpNames = append(cpNames, cacheCP.Name)
			cpNames = append(cpNames, cacheCP.FullName)
		}

		// Find any capacity pools value in this storage pool that doesn't match known capacity pools
		for _, configCP := range utils.SplitString(ctx, sPool.InternalAttributes()[capacityPools], ",") {
			if !utils.StringInSlice(configCP, cpNames) {
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

// checkForNonexistentNetworks logs warnings if any configured networks do not
// match discovered virtual networks in the resource cache.
func (c Client) checkForNonexistentNetworks(ctx context.Context) (anyMismatches bool) {
	for sPoolName, sPool := range c.sdkClient.GCNVResources.StoragePoolMap {

		// Build list of short and long capacity network names
		networkNames := make([]string, 0)
		for _, cPool := range c.sdkClient.GCNVResources.CapacityPoolMap {
			networkNames = append(networkNames, cPool.NetworkName)
			networkNames = append(networkNames, cPool.NetworkFullName)
		}

		// Find any network value in this storage pool that doesn't match the pool's network
		configNetwork := sPool.InternalAttributes()[network]
		if configNetwork != "" && !utils.StringInSlice(configNetwork, networkNames) {
			anyMismatches = true

			Logc(ctx).WithFields(LogFields{
				"pool":    sPoolName,
				"network": configNetwork,
			}).Warning("Network referenced in pool not found.")
		}
	}

	return
}

// ///////////////////////////////////////////////////////////////////////////////
// Internal functions to do discovery via the GCNV SDK
// ///////////////////////////////////////////////////////////////////////////////

// discoverCapacityPoolsWithRetry queries GCNV SDK for all capacity pools in the current location,
// retrying if the API request is throttled.
func (c Client) discoverCapacityPoolsWithRetry(ctx context.Context) (pools *[]*CapacityPool, err error) {
	discover := func() error {
		if pools, err = c.discoverCapacityPools(ctx); err != nil && IsGCNVTooManyRequestsError(err) {
			return err
		}
		return backoff.Permanent(err)
	}

	notify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration.Truncate(10 * time.Millisecond),
		}).Debugf("Retrying capacity pools query.")
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

// discoverCapacityPools queries GCNV SDK for all capacity pools in the current location.
func (c Client) discoverCapacityPools(ctx context.Context) (*[]*CapacityPool, error) {
	logFields := LogFields{
		"API": "GCNV.ListStoragePools",
	}

	var pools []*CapacityPool

	req := &netapppb.ListStoragePoolsRequest{
		Parent:   fmt.Sprintf("projects/%s/locations/%s", c.config.ProjectNumber, c.config.Location),
		PageSize: PaginationLimit,
	}
	it := c.sdkClient.gcnv.ListStoragePools(ctx, req)
	for {
		pool, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Error("Could not read pools.")
			return nil, err
		}

		_, location, capacityPool, err := parseCapacityPoolID(pool.Name)
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Warning("Skipping pool.")
		}

		_, network, err := parseNetworkID(pool.Network)
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Warning("Skipping pool.")
		}

		pools = append(pools, &CapacityPool{
			Name:            capacityPool,
			FullName:        pool.Name,
			Location:        location,
			ServiceLevel:    ServiceLevelFromGCNVServiceLevel(pool.ServiceLevel),
			State:           StoragePoolStateFromGCNVState(pool.State),
			NetworkName:     network,
			NetworkFullName: pool.Network,
		})
	}

	return &pools, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// API functions to match/search capacity pools
// ///////////////////////////////////////////////////////////////////////////////

// CapacityPools returns a list of all discovered GCNV capacity pools.
func (c Client) CapacityPools() *[]*CapacityPool {
	var cPools []*CapacityPool

	for _, cPool := range c.sdkClient.GCNVResources.CapacityPoolMap {
		cPools = append(cPools, cPool)
	}

	return &cPools
}

// capacityPool returns a single discovered capacity pool by its short name.
func (c Client) capacityPool(cPoolName string) *CapacityPool {
	for _, cPool := range c.sdkClient.GCNVResources.CapacityPoolMap {
		if cPool.Name == cPoolName {
			return cPool
		}
	}
	return nil
}

// CapacityPoolsForStoragePools returns all discovered capacity pools matching all known storage pools,
// regardless of service levels.
func (c Client) CapacityPoolsForStoragePools(ctx context.Context) []*CapacityPool {
	// This map deduplicates cPools from multiple storage pools
	cPoolMap := make(map[*CapacityPool]bool)

	// Build deduplicated map of cPools
	for _, sPool := range c.sdkClient.StoragePoolMap {
		for _, cPool := range c.CapacityPoolsForStoragePool(ctx, sPool, "") {
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
	ctx context.Context, sPool storage.Pool, serviceLevel string,
) []*CapacityPool {
	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).WithField("storagePool", sPool.Name()).
		Tracef("Determining capacity pools for storage pool.")

	// This map tracks which capacity pools have passed the filters
	filteredCapacityPoolMap := make(map[string]bool)

	// Start with all capacity pools marked as passing the filters
	for cPoolFullName := range c.sdkClient.CapacityPoolMap {
		filteredCapacityPoolMap[cPoolFullName] = true
	}

	// If capacity pools were specified, filter out non-matching capacity pools
	cpList := utils.SplitString(ctx, sPool.InternalAttributes()[capacityPools], ",")
	if len(cpList) > 0 {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !utils.SliceContainsString(cpList, cPool.Name) && !utils.SliceContainsString(cpList, cPoolFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in capacity pools [%s].",
					cPoolFullName, cpList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// If networks were specified, filter out non-matching capacity pools
	network := sPool.InternalAttributes()[network]
	if network != "" {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if network != cPool.NetworkName && network != cPool.NetworkFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in capacity pools [%s].",
					cPoolFullName, cpList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
		}
	}

	// Filter out pools with non-matching service levels
	if serviceLevel != "" {
		for cPoolFullName, cPool := range c.sdkClient.CapacityPoolMap {
			if !strings.EqualFold(cPool.ServiceLevel, serviceLevel) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not service level %s.",
					cPoolFullName, serviceLevel)
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
func (c Client) EnsureVolumeInValidCapacityPool(ctx context.Context, volume *Volume) error {
	// Get a list of all capacity pools referenced in the config
	allCapacityPools := c.CapacityPoolsForStoragePools(ctx)

	// If we aren't restricting capacity pools, any capacity pool is OK
	if len(allCapacityPools) == 0 {
		return nil
	}

	// Always match by capacity pool full name
	cPoolFullName := c.createCapacityPoolID(volume.CapacityPool)

	for _, cPool := range allCapacityPools {
		if cPoolFullName == cPool.FullName {
			return nil
		}
	}

	return errors.NotFoundError("volume %s is part of another storage pool not referenced "+
		"by this backend", volume.Name)
}
