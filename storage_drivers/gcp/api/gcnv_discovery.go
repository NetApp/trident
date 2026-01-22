// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Google Cloud NetApp Volumes SDK
package api

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/netapp/apiv1/netapppb"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"
	"google.golang.org/api/iterator"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
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
	stale, resourceUpdater, unlocker := c.sdkClient.resources.LockAndCheckStale(c.config.MaxCacheAge)

	if !stale {
		Logc(ctx).Debugf("Cached resources not yet %v old, skipping refresh.", c.config.MaxCacheAge)
		unlocker()
		return nil
	}

	// (re-)Discover what we have to work with in GCNV
	Logc(ctx).Debugf("Discovering GCNV resources.")
	discoveryErr := multierr.Combine(c.DiscoverGCNVResources(ctx, resourceUpdater))
	// Concurrent operations may continue after this point even if discovery failed
	unlocker()

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

// DiscoverGCNVResources rediscovers the GCNV resources we care about and updates the cache. Caller must hold the client's resources lock.
func (c Client) DiscoverGCNVResources(ctx context.Context, resourceUpdater GCNVResourceUpdater) (returnError error) {
	// Start from scratch each time we are called.
	newCapacityPools := make(map[string]*CapacityPool)

	defer func() {
		if returnError != nil {
			Logc(ctx).WithError(returnError).Debug("Discovery error, not retaining any discovered resources.")
			return
		}

		// Swap the newly discovered resources into the cache only if discovery succeeded.
		resourceUpdater(time.Now(), newCapacityPools)
		Logc(ctx).Debug("Switched to newly discovered resources.")
	}()

	// Discover capacity pools
	cPools, returnError := c.discoverCapacityPoolsWithRetry(ctx)
	if returnError != nil {
		return
	}

	// Update maps with all data from discovered capacity pools
	for _, cPool := range *cPools {
		newCapacityPools[cPool.FullName] = cPool
	}

	// Detect the lack of any resources: can occur when no connectivity, etc.
	// Would like a better way of proactively finding out there is something wrong
	// at a very basic level.  (Reproduce this by turning off your network!)
	numCapacityPools := len(newCapacityPools)

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

	c.sdkClient.resources.GetCapacityPools().Range(func(_ string, cp *CapacityPool) bool {
		Logd(ctx, driverName, discoveryTraceEnabled).Tracef("CPool: %s, [%s, %s]",
			cp.Name, cp.ServiceLevel, cp.NetworkName)
		return true
	})
}

// checkForUnsatisfiedPools returns one or more errors if one or more configured storage pools
// are satisfied by no capacity pools.
func (c Client) checkForUnsatisfiedPools(ctx context.Context) (discoveryErrors []error) {
	// Ensure every storage pool matches one or more capacity pools
	c.sdkClient.resources.GetStoragePools().Range(func(sPoolName string, sPool storage.Pool) bool {

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
		return true
	})

	return
}

// checkForNonexistentCapacityPools logs warnings if any configured capacity pools do not
// match discovered capacity pools in the resource cache.
func (c Client) checkForNonexistentCapacityPools(ctx context.Context) (anyMismatches bool) {
	cPools := c.sdkClient.resources.GetCapacityPools()
	cpNames := make([]string, 0, cPools.Length()*2)
	cPools.Range(func(_ string, cp *CapacityPool) bool {
		cpNames = append(cpNames, cp.Name)
		cpNames = append(cpNames, cp.FullName)
		return true
	})
	c.sdkClient.resources.GetStoragePools().Range(func(sPoolName string, sPool storage.Pool) bool {

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
		return true
	})

	return
}

// checkForNonexistentNetworks logs warnings if any configured networks do not
// match discovered virtual networks in the resource cache.
func (c Client) checkForNonexistentNetworks(ctx context.Context) (anyMismatches bool) {
	// Build list of short and long capacity network names
	cPools := c.sdkClient.resources.GetCapacityPools()
	networkNames := make([]string, 0, cPools.Length()*2)
	cPools.Range(func(_ string, cp *CapacityPool) bool {
		networkNames = append(networkNames, cp.NetworkName)
		networkNames = append(networkNames, cp.NetworkFullName)
		return true
	})
	c.sdkClient.resources.GetStoragePools().Range(func(sPoolName string, sPool storage.Pool) bool {

		// Find any network value in this storage pool that doesn't match the pool's network
		configNetwork := sPool.InternalAttributes()[network]
		if configNetwork != "" && !collection.StringInSlice(configNetwork, networkNames) {
			anyMismatches = true

			Logc(ctx).WithFields(LogFields{
				"pool":    sPoolName,
				"network": configNetwork,
			}).Warning("Network referenced in pool not found.")
		}
		return true
	})

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
	var locations []string
	locations = append(locations, c.config.Location)

	flexPoolsCount := c.flexPoolCount()
	if flexPoolsCount != 0 {
		locations = append(locations, c.ListComputeZones(ctx)...)
	}

	var pools []*CapacityPool
	// We iterate over a list of region and zones to get the storage pools
	// created in those regions and zones.
	for _, location := range locations {
		req := &netapppb.ListStoragePoolsRequest{
			Parent:   fmt.Sprintf("projects/%s/locations/%s", c.config.ProjectNumber, location),
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
				break
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
				Zone:            pool.Zone,
			})
		}
	}

	if len(pools) == 0 {
		return nil, errors.NotFoundError("no capacity pools found for the given region")
	}
	return &pools, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// API functions to match/search capacity pools
// ///////////////////////////////////////////////////////////////////////////////

// CapacityPools returns a list of all discovered GCNV capacity pools.
func (c Client) CapacityPools() *[]*CapacityPool {
	cPoolsMap := c.sdkClient.resources.GetCapacityPools()
	cPools := make([]*CapacityPool, 0, cPoolsMap.Length())
	cPoolsMap.Range(func(_ string, cPool *CapacityPool) bool {
		cPools = append(cPools, cPool)
		return true
	})

	return &cPools
}

// capacityPool returns a single discovered capacity pool by its short name.
func (c Client) capacityPool(cPoolName string) *CapacityPool {
	var matchingCPool *CapacityPool
	c.sdkClient.resources.GetCapacityPools().Range(func(_ string, cPool *CapacityPool) bool {
		if cPool.Name == cPoolName {
			matchingCPool = cPool
			return false
		}
		return true
	})
	return matchingCPool
}

// CapacityPoolsForStoragePools returns all discovered capacity pools matching all known storage pools,
// regardless of service levels.
func (c Client) CapacityPoolsForStoragePools(ctx context.Context) []*CapacityPool {
	// This map deduplicates cPools from multiple storage pools
	cPoolMap := make(map[*CapacityPool]bool)

	// Build deduplicated map of cPools
	c.sdkClient.resources.GetStoragePools().Range(func(_ string, sPool storage.Pool) bool {
		for _, cPool := range c.CapacityPoolsForStoragePool(ctx, sPool, "") {
			cPoolMap[cPool] = true
		}
		return true
	})

	// Copy keys into a list of deduplicated cPools
	cPools := make([]*CapacityPool, 0, len(cPoolMap))

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
	cPools := c.sdkClient.resources.GetCapacityPools()
	// Start with all capacity pools marked as passing the filters
	cPools.Range(func(cPoolFullName string, _ *CapacityPool) bool {
		filteredCapacityPoolMap[cPoolFullName] = true
		return true
	})

	// If capacity pools were specified, filter out non-matching capacity pools
	cpList := collection.SplitString(ctx, sPool.InternalAttributes()[capacityPools], ",")
	if len(cpList) > 0 {
		cPools.Range(func(cPoolFullName string, cPool *CapacityPool) bool {
			if !collection.ContainsString(cpList, cPool.Name) && !collection.ContainsString(cpList, cPoolFullName) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in capacity pools [%s].",
					cPoolFullName, cpList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
			return true
		})
	}

	// If networks were specified, filter out non-matching capacity pools
	network := sPool.InternalAttributes()[network]
	if network != "" {
		cPools.Range(func(cPoolFullName string, cPool *CapacityPool) bool {
			if network != cPool.NetworkName && network != cPool.NetworkFullName {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not in capacity pools [%s].",
					cPoolFullName, cpList)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
			return true
		})
	}

	// Filter out pools with non-matching service levels
	if serviceLevel != "" {
		cPools.Range(func(cPoolFullName string, cPool *CapacityPool) bool {
			if !strings.EqualFold(cPool.ServiceLevel, serviceLevel) {
				Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef("Ignoring capacity pool %s, not service level %s.",
					cPoolFullName, serviceLevel)
				filteredCapacityPoolMap[cPoolFullName] = false
			}
			return true
		})
	}

	// Build list of all capacity pools that have passed all filters
	filteredCPools := make([]*CapacityPool, 0, len(filteredCapacityPoolMap))
	for cPoolFullName, match := range filteredCapacityPoolMap {
		if match {
			filteredCPools = append(filteredCPools, cPools.Get(cPoolFullName))
		}
	}

	// Filter out Capacity pools with non-matching supported topology
	filteredCPools = c.FilterCapacityPoolsOnTopology(ctx, filteredCPools, sPool.SupportedTopologies(), []map[string]string{})

	return filteredCPools
}

// FilterCapacityPoolsOnTopology returns all the discovered capacity pools that support any of the requisite
// topologies. The discovered capacity pools are sorted based on the preferred topologies.
func (c Client) FilterCapacityPoolsOnTopology(
	ctx context.Context, cPools []*CapacityPool, requisiteTopologies, preferredTopologies []map[string]string,
) []*CapacityPool {
	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["discovery"]).Tracef(
		"Determining capacity pools for requisite topologies.")

	if len(requisiteTopologies) == 0 {
		return cPools
	}

	filteredCPools := make([]*CapacityPool, 0)
	// Filtering pools on requisite topology
	for _, cPool := range cPools {
		if cPool.Zone != "" {
			for _, topology := range requisiteTopologies {
				// If region and zone are not specified in the topology, we assume that the capacity pool
				// is a regional capacity pool and is always assumed to match the requisite topology.
				region, zone := sc.GetRegionZoneForTopology(topology)
				if (zone == "" || strings.EqualFold(cPool.Zone, zone)) && (region == "" ||
					strings.EqualFold(cPool.Location, region)) {
					filteredCPools = append(filteredCPools, cPool)
				} else if strings.EqualFold(cPool.Zone, zone) && strings.EqualFold(cPool.Location, zone) {
					// If location and zone of a capacity pool are same, we assume that the capacity pool is
					// a zonal capacity pool and is matched with the zone specified in the topology.
					filteredCPools = append(filteredCPools, cPool)
				}
			}
		} else {
			// A capacity pool without a zone is considered to be a regional capacity pool, and is always
			// assumed to match the requisite topology.
			filteredCPools = append(filteredCPools, cPool)
		}
	}

	if len(preferredTopologies) > 0 {
		filteredCPools = SortCPoolsByPreferredTopologies(ctx, filteredCPools, preferredTopologies)
	}

	return filteredCPools
}

// SortCPoolsByPreferredTopologies returns a list of capacity pools ordered by the provided list of preferred
// topologies.
func SortCPoolsByPreferredTopologies(
	ctx context.Context, cPools []*CapacityPool, preferredTopologies []map[string]string,
) []*CapacityPool {
	orderedCPools := make([]*CapacityPool, 0)

	rand.Shuffle(len(cPools), func(i, j int) {
		cPools[i], cPools[j] = cPools[j], cPools[i]
	})

	for _, preferred := range preferredTopologies {

		remainingCPools := make([]*CapacityPool, 0)

		for _, cPool := range cPools {
			// If it supports topology, pop it and add to orderedcpool. Otherwise, add it to remaining
			// capacity pools to be addressed in future loop iterations.
			_, zone := sc.GetRegionZoneForTopology(preferred)
			if strings.EqualFold(cPool.Zone, zone) {
				orderedCPools = append(orderedCPools, cPool)
			} else {
				remainingCPools = append(remainingCPools, cPool)
			}
		}

		// make new list out of remaining capacity pools
		cPools = remainingCPools
	}

	return append(orderedCPools, cPools...)
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
	cPoolFullName := c.createCapacityPoolID(volume.Location, volume.CapacityPool)

	for _, cPool := range allCapacityPools {
		if cPoolFullName == cPool.FullName {
			return nil
		}
	}

	return errors.NotFoundError("volume %s is part of another storage pool not referenced "+
		"by this backend", volume.Name)
}

func (c Client) ListComputeZones(ctx context.Context) []string {
	logFields := LogFields{
		"API": "Compute.List",
	}
	var locations []string
	req1 := &computepb.ListRegionZonesRequest{
		Project: c.config.ProjectNumber,
		Region:  c.config.Location,
	}
	zoneIterator := c.sdkClient.compute.List(ctx, req1)
	for {
		zone, err := zoneIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Error("Could not read zones.")
			break
		}
		locations = append(locations, *zone.Name)
	}
	return locations
}
