// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAutogrowCache(t *testing.T) {
	cache := NewAutogrowCache()

	assert.NotNil(t, cache)
	assert.NotNil(t, cache.effectivePolicyCache)

	// Verify cache is empty
	assert.False(t, cache.HasEffectivePolicyName("test"))
}

// ============================================================================
// Effective Policy Cache Operations Tests (TVP → Policy Name)
// ============================================================================

func TestSetEffectivePolicyName(t *testing.T) {
	cache := NewAutogrowCache()

	policyName := "testPolicy"
	tvpName := "vol1.node1"

	err := cache.SetEffectivePolicyName(tvpName, policyName)
	assert.NoError(t, err)

	// Verify the policy name was cached
	cachedPolicyName, err := cache.GetEffectivePolicyName(tvpName)
	assert.NoError(t, err)
	assert.Equal(t, policyName, cachedPolicyName)
}

func TestSetEffectivePolicyName_EmptyTVPName(t *testing.T) {
	cache := NewAutogrowCache()

	err := cache.SetEffectivePolicyName("", "policy")
	assert.Error(t, err, "empty tvp name should error")
}

func TestSetEffectivePolicyName_Overwrite(t *testing.T) {
	cache := NewAutogrowCache()

	tvpName := "vol1.node1"

	// Set initial policy
	err := cache.SetEffectivePolicyName(tvpName, "policy1")
	assert.NoError(t, err)

	// Overwrite with new policy
	err = cache.SetEffectivePolicyName(tvpName, "policy2")
	assert.NoError(t, err)

	// Should return the new policy
	cachedPolicyName, err := cache.GetEffectivePolicyName(tvpName)
	assert.NoError(t, err)
	assert.Equal(t, "policy2", cachedPolicyName)
}

func TestGetEffectivePolicyName(t *testing.T) {
	cache := NewAutogrowCache()

	// Get non-existent policy
	policyName, err := cache.GetEffectivePolicyName("nonexistent.node1")
	assert.Error(t, err)
	assert.Equal(t, "", policyName)

	// Set and get policy name
	tvpName := "vol1.node1"
	testPolicyName := "testPolicy"
	cache.SetEffectivePolicyName(tvpName, testPolicyName)

	policyName, err = cache.GetEffectivePolicyName(tvpName)
	assert.NoError(t, err)
	assert.Equal(t, testPolicyName, policyName)
}

func TestHasEffectivePolicyName(t *testing.T) {
	cache := NewAutogrowCache()

	tvpName := "vol1.node1"

	// Non-existent tvp
	assert.False(t, cache.HasEffectivePolicyName("nonexistent.node1"))

	// After setting policy name
	cache.SetEffectivePolicyName(tvpName, "testPolicy")
	assert.True(t, cache.HasEffectivePolicyName(tvpName))

	// After deleting policy name
	cache.DeleteEffectivePolicyName(tvpName)
	assert.False(t, cache.HasEffectivePolicyName(tvpName))
}

// ============================================================================
// Utility Operations Tests
// ============================================================================

func TestDeleteTVP(t *testing.T) {
	cache := NewAutogrowCache()

	tvpName := "vol1.node1"

	// Set up tvp data
	cache.SetEffectivePolicyName(tvpName, "testPolicy")

	// Delete tvp
	err := cache.DeleteTVP(tvpName)
	assert.NoError(t, err)

	// Effective policy should be deleted
	assert.False(t, cache.HasEffectivePolicyName(tvpName))
}

func TestDeleteTVP_MultipleTVPs(t *testing.T) {
	cache := NewAutogrowCache()

	// Set up multiple tvps
	tvp1 := "vol1.node1"
	tvp2 := "vol2.node1"
	tvp3 := "vol3.node1"

	cache.SetEffectivePolicyName(tvp1, "policy1")
	cache.SetEffectivePolicyName(tvp2, "policy2")
	cache.SetEffectivePolicyName(tvp3, "policy3")

	// Delete middle tvp
	err := cache.DeleteTVP(tvp2)
	assert.NoError(t, err)

	// tvp2 should be gone from cache
	assert.False(t, cache.HasEffectivePolicyName(tvp2))

	// Other tvps should be unaffected
	assert.True(t, cache.HasEffectivePolicyName(tvp1))
	assert.True(t, cache.HasEffectivePolicyName(tvp3))
}

func TestClear(t *testing.T) {
	cache := NewAutogrowCache()

	tvp1 := "vol1.node1"
	tvp2 := "vol2.node1"
	tvp5 := "vol5.node1"

	// Set up data
	cache.SetEffectivePolicyName(tvp1, "testPolicy")
	cache.SetEffectivePolicyName(tvp2, "testPolicy")

	// Clear all data
	cache.Clear()

	// Verify all data is cleared through public API
	assert.False(t, cache.HasEffectivePolicyName(tvp1))
	assert.False(t, cache.HasEffectivePolicyName(tvp2))

	// Verify cache still works after clear (can add new data)
	err := cache.SetEffectivePolicyName(tvp5, "newPolicy")
	assert.NoError(t, err)
	assert.True(t, cache.HasEffectivePolicyName(tvp5))
}

// ============================================================================
// Concurrency Tests
// ============================================================================

func TestConcurrentAccess_EffectivePolicy(t *testing.T) {
	cache := NewAutogrowCache()
	done := make(chan bool)
	policy := "testPolicy"
	tvp1 := "vol1.node1"
	tvp2 := "vol2.node1"
	tvp3 := "vol3.node1"

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			cache.SetEffectivePolicyName(tvp1, policy)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			cache.SetEffectivePolicyName(tvp2, policy)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetEffectivePolicyName(tvp1)
			cache.HasEffectivePolicyName(tvp2)
		}
		done <- true
	}()

	// Concurrent deletes
	go func() {
		for i := 0; i < 100; i++ {
			cache.DeleteEffectivePolicyName(tvp3)
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
	<-done
}

func TestConcurrentAccess_Mixed(t *testing.T) {
	cache := NewAutogrowCache()
	done := make(chan bool)
	tvpName := "vol1.node1"

	// Mixed operations
	go func() {
		for i := 0; i < 50; i++ {
			cache.SetEffectivePolicyName(tvpName, "testPolicy")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			cache.DeleteEffectivePolicyName(tvpName)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			cache.GetEffectivePolicyName(tvpName)
			cache.HasEffectivePolicyName(tvpName)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			cache.Clear()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
	<-done
}
