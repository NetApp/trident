// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cache

import (
	"fmt"

	"github.com/netapp/trident/pkg/cache"
	"github.com/netapp/trident/pkg/cache/generic_cache"
)

// AutogrowCache provides thread-safe caching for the Autogrow feature at the node level.
// It maintains the effective policy mapping: TVP Name → Policy Name (for quick policy lookups)
//
// Important: All cache operations use tvpName as the identifier.
// TVP name format is volumeID.nodeID (e.g., pvc-123.node1)
// This provides consistency across all Autogrow layers
type AutogrowCache struct {
	// Effective policy cache: tvpName → policy name
	// Stores the policy name for each TVP (after hierarchy resolution)
	// The actual policy spec should be retrieved from the AGP informer using this name
	effectivePolicyCache cache.Cache[string, string]
}

// NewAutogrowCache creates a new Autogrow feature cache
func NewAutogrowCache() *AutogrowCache {
	return &AutogrowCache{
		effectivePolicyCache: generic_cache.NewSingleValueCache[string, string](),
	}
}

// ============================================================================
// Effective Policy Cache Operations (TVP Name → Policy Name)
// ============================================================================

// GetEffectivePolicyName retrieves the cached effective Autogrow policy name for a TVP.
// Returns error if no policy has been cached yet.
// The caller should use this name to lookup the actual policy from the AGP informer.
// tvpName parameter should be the TridentVolumePublication name (volumeID.nodeID format).
func (c *AutogrowCache) GetEffectivePolicyName(tvpName string) (string, error) {
	values := c.effectivePolicyCache.Get(tvpName)
	if len(values) == 0 {
		return "", fmt.Errorf("no effective policy cached for tvp: %s", tvpName)
	}

	return values[0], nil
}

// SetEffectivePolicyName caches the effective Autogrow policy name for a TVP.
// This should be called by the policy resolution/scheduler layer after computing the final policy.
// Pass empty string to explicitly cache "no policy" (different from "not yet computed").
// tvpName parameter should be the TridentVolumePublication name (volumeID.nodeID format).
func (c *AutogrowCache) SetEffectivePolicyName(tvpName, policyName string) error {
	return c.effectivePolicyCache.Set(tvpName, policyName)
}

// DeleteEffectivePolicyName removes the cached effective policy name for a TVP.
// Called when:
// - TVP is deleted
// - Policy needs recalculation (invalidation)
// tvpName parameter should be the TridentVolumePublication name (volumeID.nodeID format).
func (c *AutogrowCache) DeleteEffectivePolicyName(tvpName string) error {
	return c.effectivePolicyCache.DeleteKey(tvpName)
}

// HasEffectivePolicyName checks if an effective policy name is cached for a TVP.
// Useful for quick checks before attempting resolution.
// tvpName parameter should be the TridentVolumePublication name (volumeID.nodeID format).
func (c *AutogrowCache) HasEffectivePolicyName(tvpName string) bool {
	return c.effectivePolicyCache.Has(tvpName)
}

// ============================================================================
// Utility Operations
// ============================================================================

// DeleteTVP removes all cached data for a TVP.
// Call this when a TVP is deleted or unpublished from the node.
// tvpName parameter should be the TridentVolumePublication name (volumeID.nodeID format).
func (c *AutogrowCache) DeleteTVP(tvpName string) error {
	// Delete effective policy cache entry
	_ = c.effectivePolicyCache.DeleteKey(tvpName)
	return nil
}

// Clear removes all cached data (for testing or full reset).
func (c *AutogrowCache) Clear() {
	c.effectivePolicyCache.Clear()
}
