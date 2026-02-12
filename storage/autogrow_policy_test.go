// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Constructor Tests

func TestNewAutogrowPolicy(t *testing.T) {
	tests := map[string]struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
		state         AutogrowPolicyState
		expectedState AutogrowPolicyState
	}{
		"Create policy with all fields": {
			name:          "test-policy",
			usedThreshold: "80",
			growthAmount:  "20",
			maxSize:       "1000",
			state:         AutogrowPolicyStateSuccess,
			expectedState: AutogrowPolicyStateSuccess,
		},
		"Create policy with empty state defaults to Success": {
			name:          "test-policy-2",
			usedThreshold: "90",
			growthAmount:  "10",
			maxSize:       "2000",
			state:         "",
			expectedState: AutogrowPolicyStateSuccess,
		},
		"Create policy with Failed state": {
			name:          "test-policy-3",
			usedThreshold: "75",
			growthAmount:  "25",
			maxSize:       "5000",
			state:         AutogrowPolicyStateFailed,
			expectedState: AutogrowPolicyStateFailed,
		},
		"Create policy with Deleting state": {
			name:          "test-policy-4",
			usedThreshold: "85",
			growthAmount:  "15",
			maxSize:       "3000",
			state:         AutogrowPolicyStateDeleting,
			expectedState: AutogrowPolicyStateDeleting,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			policy := NewAutogrowPolicy(test.name, test.usedThreshold, test.growthAmount, test.maxSize, test.state)

			assert.NotNil(t, policy, "Policy should not be nil")
			assert.Equal(t, test.name, policy.Name(), "Policy name mismatch")
			assert.Equal(t, test.usedThreshold, policy.UsedThreshold(), "UsedThreshold mismatch")
			assert.Equal(t, test.growthAmount, policy.GrowthAmount(), "GrowthAmount mismatch")
			assert.Equal(t, test.maxSize, policy.MaxSize(), "MaxSize mismatch")
			assert.Equal(t, test.expectedState, policy.State(), "State mismatch")
			assert.NotNil(t, policy.volumes, "Volumes map should be initialized")
			assert.NotNil(t, policy.mutex, "Mutex should be initialized")
		})
	}
}

func TestNewAutogrowPolicyFromConfig(t *testing.T) {
	tests := map[string]struct {
		config *AutogrowPolicyConfig
	}{
		"Create from config with all fields": {
			config: &AutogrowPolicyConfig{
				Name:          "config-policy",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000",
				State:         AutogrowPolicyStateSuccess,
			},
		},
		"Create from config with empty state": {
			config: &AutogrowPolicyConfig{
				Name:          "config-policy-2",
				UsedThreshold: "90",
				GrowthAmount:  "10",
				MaxSize:       "2000",
				State:         "",
			},
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			policy := NewAutogrowPolicyFromConfig(test.config)

			assert.NotNil(t, policy, "Policy should not be nil")
			assert.Equal(t, test.config.Name, policy.Name(), "Policy name mismatch")
			assert.Equal(t, test.config.UsedThreshold, policy.UsedThreshold(), "UsedThreshold mismatch")
			assert.Equal(t, test.config.GrowthAmount, policy.GrowthAmount(), "GrowthAmount mismatch")
			assert.Equal(t, test.config.MaxSize, policy.MaxSize(), "MaxSize mismatch")
		})
	}
}

func TestNewAutogrowPolicyFromPersistent(t *testing.T) {
	tests := map[string]struct {
		persistent *AutogrowPolicyPersistent
	}{
		"Create from persistent with all fields": {
			persistent: &AutogrowPolicyPersistent{
				Name:          "persistent-policy",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000",
				State:         AutogrowPolicyStateSuccess,
			},
		},
		"Create from persistent with Failed state": {
			persistent: &AutogrowPolicyPersistent{
				Name:          "persistent-policy-2",
				UsedThreshold: "90",
				GrowthAmount:  "10",
				MaxSize:       "2000",
				State:         AutogrowPolicyStateFailed,
			},
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			policy := NewAutogrowPolicyFromPersistent(test.persistent)

			assert.NotNil(t, policy, "Policy should not be nil")
			assert.Equal(t, test.persistent.Name, policy.Name(), "Policy name mismatch")
			assert.Equal(t, test.persistent.UsedThreshold, policy.UsedThreshold(), "UsedThreshold mismatch")
			assert.Equal(t, test.persistent.GrowthAmount, policy.GrowthAmount(), "GrowthAmount mismatch")
			assert.Equal(t, test.persistent.MaxSize, policy.MaxSize(), "MaxSize mismatch")
			assert.Equal(t, test.persistent.State, policy.State(), "State mismatch")
		})
	}
}

// State Helper Tests

func TestAutogrowPolicyState_IsSuccess(t *testing.T) {
	tests := map[string]struct {
		state    AutogrowPolicyState
		expected bool
	}{
		"Success state":  {state: AutogrowPolicyStateSuccess, expected: true},
		"Failed state":   {state: AutogrowPolicyStateFailed, expected: false},
		"Deleting state": {state: AutogrowPolicyStateDeleting, expected: false},
		"Empty state":    {state: "", expected: false},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := test.state.IsSuccess()
			assert.Equal(t, test.expected, result, "IsSuccess result mismatch")
		})
	}
}

func TestAutogrowPolicyState_IsFailed(t *testing.T) {
	tests := map[string]struct {
		state    AutogrowPolicyState
		expected bool
	}{
		"Success state":  {state: AutogrowPolicyStateSuccess, expected: false},
		"Failed state":   {state: AutogrowPolicyStateFailed, expected: true},
		"Deleting state": {state: AutogrowPolicyStateDeleting, expected: false},
		"Empty state":    {state: "", expected: false},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := test.state.IsFailed()
			assert.Equal(t, test.expected, result, "IsFailed result mismatch")
		})
	}
}

func TestAutogrowPolicyState_IsDeleting(t *testing.T) {
	tests := map[string]struct {
		state    AutogrowPolicyState
		expected bool
	}{
		"Success state":  {state: AutogrowPolicyStateSuccess, expected: false},
		"Failed state":   {state: AutogrowPolicyStateFailed, expected: false},
		"Deleting state": {state: AutogrowPolicyStateDeleting, expected: true},
		"Empty state":    {state: "", expected: false},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := test.state.IsDeleting()
			assert.Equal(t, test.expected, result, "IsDeleting result mismatch")
		})
	}
}

// Getter Method Tests

func TestAutogrowPolicy_Name(t *testing.T) {
	policy := NewAutogrowPolicy("test-name", "80", "20", "1000", AutogrowPolicyStateSuccess)
	assert.Equal(t, "test-name", policy.Name(), "Name mismatch")
}

func TestAutogrowPolicy_UsedThreshold(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "85", "20", "1000", AutogrowPolicyStateSuccess)
	assert.Equal(t, "85", policy.UsedThreshold(), "UsedThreshold mismatch")
}

func TestAutogrowPolicy_GrowthAmount(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "25", "1000", AutogrowPolicyStateSuccess)
	assert.Equal(t, "25", policy.GrowthAmount(), "GrowthAmount mismatch")
}

func TestAutogrowPolicy_MaxSize(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "5000", AutogrowPolicyStateSuccess)
	assert.Equal(t, "5000", policy.MaxSize(), "MaxSize mismatch")
}

func TestAutogrowPolicy_State(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateFailed)
	assert.Equal(t, AutogrowPolicyStateFailed, policy.State(), "State mismatch")
}

// Config Method Tests

func TestAutogrowPolicy_GetConfig(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	config := policy.GetConfig()

	assert.NotNil(t, config, "Config should not be nil")
	assert.Equal(t, "test-policy", config.Name, "Name mismatch")
	assert.Equal(t, "80", config.UsedThreshold, "UsedThreshold mismatch")
	assert.Equal(t, "20", config.GrowthAmount, "GrowthAmount mismatch")
	assert.Equal(t, "1000", config.MaxSize, "MaxSize mismatch")
	assert.Equal(t, AutogrowPolicyStateSuccess, config.State, "State mismatch")
}

func TestAutogrowPolicy_UpdateFromConfig(t *testing.T) {
	tests := map[string]struct {
		initialPolicy *AutogrowPolicy
		updateConfig  *AutogrowPolicyConfig
		expectedState AutogrowPolicyState
	}{
		"Update all fields with state": {
			initialPolicy: NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			updateConfig: &AutogrowPolicyConfig{
				Name:          "test-policy",
				UsedThreshold: "90",
				GrowthAmount:  "30",
				MaxSize:       "2000",
				State:         AutogrowPolicyStateFailed,
			},
			expectedState: AutogrowPolicyStateFailed,
		},
		"Update fields without state": {
			initialPolicy: NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			updateConfig: &AutogrowPolicyConfig{
				Name:          "test-policy",
				UsedThreshold: "85",
				GrowthAmount:  "25",
				MaxSize:       "1500",
				State:         "",
			},
			expectedState: AutogrowPolicyStateSuccess,
		},
		"Update to Deleting state": {
			initialPolicy: NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			updateConfig: &AutogrowPolicyConfig{
				Name:          "test-policy",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000",
				State:         AutogrowPolicyStateDeleting,
			},
			expectedState: AutogrowPolicyStateDeleting,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			test.initialPolicy.UpdateFromConfig(test.updateConfig)

			assert.Equal(t, test.updateConfig.UsedThreshold, test.initialPolicy.UsedThreshold(), "UsedThreshold mismatch")
			assert.Equal(t, test.updateConfig.GrowthAmount, test.initialPolicy.GrowthAmount(), "GrowthAmount mismatch")
			assert.Equal(t, test.updateConfig.MaxSize, test.initialPolicy.MaxSize(), "MaxSize mismatch")
			assert.Equal(t, test.expectedState, test.initialPolicy.State(), "State mismatch")
		})
	}
}

// Volume Association Tests

func TestAutogrowPolicy_AddVolume(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	policy.AddVolume("volume-1")
	assert.True(t, policy.HasVolume("volume-1"), "Volume should be added")

	policy.AddVolume("volume-2")
	assert.True(t, policy.HasVolume("volume-2"), "Volume should be added")

	// Add same volume again
	policy.AddVolume("volume-1")
	assert.True(t, policy.HasVolume("volume-1"), "Volume should still exist")
}

func TestAutogrowPolicy_RemoveVolume(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	policy.AddVolume("volume-1")
	policy.AddVolume("volume-2")

	policy.RemoveVolume("volume-1")
	assert.False(t, policy.HasVolume("volume-1"), "Volume should be removed")
	assert.True(t, policy.HasVolume("volume-2"), "Volume-2 should still exist")

	// Remove non-existent volume
	policy.RemoveVolume("non-existent")
	assert.False(t, policy.HasVolume("non-existent"), "Non-existent volume should not exist")
}

func TestAutogrowPolicy_HasVolume(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	assert.False(t, policy.HasVolume("volume-1"), "Volume should not exist initially")

	policy.AddVolume("volume-1")
	assert.True(t, policy.HasVolume("volume-1"), "Volume should exist after adding")

	policy.RemoveVolume("volume-1")
	assert.False(t, policy.HasVolume("volume-1"), "Volume should not exist after removing")
}

func TestAutogrowPolicy_HasVolumes(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	assert.False(t, policy.HasVolumes(), "Should have no volumes initially")

	policy.AddVolume("volume-1")
	assert.True(t, policy.HasVolumes(), "Should have volumes after adding")

	policy.AddVolume("volume-2")
	assert.True(t, policy.HasVolumes(), "Should still have volumes")

	policy.RemoveVolume("volume-1")
	assert.True(t, policy.HasVolumes(), "Should still have volumes")

	policy.RemoveVolume("volume-2")
	assert.False(t, policy.HasVolumes(), "Should have no volumes after removing all")
}

func TestAutogrowPolicy_GetVolumes(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	volumes := policy.GetVolumes()
	assert.Empty(t, volumes, "Should have no volumes initially")

	policy.AddVolume("volume-1")
	policy.AddVolume("volume-2")
	policy.AddVolume("volume-3")

	volumes = policy.GetVolumes()
	assert.Len(t, volumes, 3, "Should have 3 volumes")
	assert.Contains(t, volumes, "volume-1", "Should contain volume-1")
	assert.Contains(t, volumes, "volume-2", "Should contain volume-2")
	assert.Contains(t, volumes, "volume-3", "Should contain volume-3")
}

func TestAutogrowPolicy_VolumeCount(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	assert.Equal(t, 0, policy.VolumeCount(), "Should have 0 volumes initially")

	policy.AddVolume("volume-1")
	assert.Equal(t, 1, policy.VolumeCount(), "Should have 1 volume")

	policy.AddVolume("volume-2")
	assert.Equal(t, 2, policy.VolumeCount(), "Should have 2 volumes")

	policy.AddVolume("volume-3")
	assert.Equal(t, 3, policy.VolumeCount(), "Should have 3 volumes")

	policy.RemoveVolume("volume-2")
	assert.Equal(t, 2, policy.VolumeCount(), "Should have 2 volumes after removal")
}

func TestAutogrowPolicy_ClearVolumes(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	policy.AddVolume("volume-1")
	policy.AddVolume("volume-2")
	policy.AddVolume("volume-3")

	assert.Equal(t, 3, policy.VolumeCount(), "Should have 3 volumes")

	policy.ClearVolumes()
	assert.Equal(t, 0, policy.VolumeCount(), "Should have 0 volumes after clearing")
	assert.False(t, policy.HasVolumes(), "Should have no volumes after clearing")
	assert.False(t, policy.HasVolume("volume-1"), "volume-1 should not exist")
}

// External Representation Tests

func TestAutogrowPolicy_ConstructExternal(t *testing.T) {
	tests := map[string]struct {
		policy        *AutogrowPolicy
		volumesToAdd  []string
		expectedCount int
	}{
		"Policy with no volumes": {
			policy:        NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			volumesToAdd:  []string{},
			expectedCount: 0,
		},
		"Policy with one volume": {
			policy:        NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			volumesToAdd:  []string{"volume-1"},
			expectedCount: 1,
		},
		"Policy with multiple volumes": {
			policy:        NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			volumesToAdd:  []string{"volume-1", "volume-2", "volume-3"},
			expectedCount: 3,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			for _, vol := range test.volumesToAdd {
				test.policy.AddVolume(vol)
			}

			ctx := context.Background()
			external := test.policy.ConstructExternal(ctx)

			assert.NotNil(t, external, "External should not be nil")
			assert.Equal(t, test.policy.Name(), external.Name, "Name mismatch")
			assert.Equal(t, test.policy.UsedThreshold(), external.UsedThreshold, "UsedThreshold mismatch")
			assert.Equal(t, test.policy.GrowthAmount(), external.GrowthAmount, "GrowthAmount mismatch")
			assert.Equal(t, test.policy.MaxSize(), external.MaxSize, "MaxSize mismatch")
			assert.Equal(t, test.policy.State(), external.State, "State mismatch")
			assert.Equal(t, test.expectedCount, external.VolumeCount, "VolumeCount mismatch")
			assert.Len(t, external.Volumes, test.expectedCount, "Volumes length mismatch")

			for _, vol := range test.volumesToAdd {
				assert.Contains(t, external.Volumes, vol, "Volume should be in external representation")
			}
		})
	}
}

// Persistent Representation Tests

func TestAutogrowPolicy_ConstructPersistent(t *testing.T) {
	tests := map[string]struct {
		policy *AutogrowPolicy
	}{
		"Policy with Success state": {
			policy: NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
		},
		"Policy with Failed state": {
			policy: NewAutogrowPolicy("test-policy-2", "90", "30", "2000", AutogrowPolicyStateFailed),
		},
		"Policy with Deleting state": {
			policy: NewAutogrowPolicy("test-policy-3", "85", "25", "1500", AutogrowPolicyStateDeleting),
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			persistent := test.policy.ConstructPersistent()

			assert.NotNil(t, persistent, "Persistent should not be nil")
			assert.Equal(t, test.policy.Name(), persistent.Name, "Name mismatch")
			assert.Equal(t, test.policy.UsedThreshold(), persistent.UsedThreshold, "UsedThreshold mismatch")
			assert.Equal(t, test.policy.GrowthAmount(), persistent.GrowthAmount, "GrowthAmount mismatch")
			assert.Equal(t, test.policy.MaxSize(), persistent.MaxSize, "MaxSize mismatch")
			assert.Equal(t, test.policy.State(), persistent.State, "State mismatch")
		})
	}
}

func TestAutogrowPolicy_RoundTripPersistent(t *testing.T) {
	// Test that converting to persistent and back preserves data
	originalPolicy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	persistent := originalPolicy.ConstructPersistent()
	reconstructedPolicy := NewAutogrowPolicyFromPersistent(persistent)

	assert.Equal(t, originalPolicy.Name(), reconstructedPolicy.Name(), "Name mismatch")
	assert.Equal(t, originalPolicy.UsedThreshold(), reconstructedPolicy.UsedThreshold(), "UsedThreshold mismatch")
	assert.Equal(t, originalPolicy.GrowthAmount(), reconstructedPolicy.GrowthAmount(), "GrowthAmount mismatch")
	assert.Equal(t, originalPolicy.MaxSize(), reconstructedPolicy.MaxSize(), "MaxSize mismatch")
	assert.Equal(t, originalPolicy.State(), reconstructedPolicy.State(), "State mismatch")
}

// Concurrency Tests

func TestAutogrowPolicy_ConcurrentAccess(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	var wg sync.WaitGroup

	// Test concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = policy.Name()
			_ = policy.UsedThreshold()
			_ = policy.GrowthAmount()
			_ = policy.MaxSize()
			_ = policy.State()
		}()
	}

	wg.Wait()
}

func TestAutogrowPolicy_ConcurrentUpdates(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	var wg sync.WaitGroup

	// Test concurrent updates
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			config := &AutogrowPolicyConfig{
				Name:          "test-policy",
				UsedThreshold: "85",
				GrowthAmount:  "25",
				MaxSize:       "2000",
				State:         AutogrowPolicyStateSuccess,
			}
			policy.UpdateFromConfig(config)
		}(i)
	}

	wg.Wait()

	// Verify final state
	assert.Equal(t, "85", policy.UsedThreshold(), "UsedThreshold should be updated")
	assert.Equal(t, "25", policy.GrowthAmount(), "GrowthAmount should be updated")
	assert.Equal(t, "2000", policy.MaxSize(), "MaxSize should be updated")
}

func TestAutogrowPolicy_ConcurrentVolumeOperations(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	var wg sync.WaitGroup

	// Test concurrent volume additions
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			volumeName := "volume-" + string(rune('0'+index%10))
			policy.AddVolume(volumeName)
		}(i)
	}

	wg.Wait()

	assert.True(t, policy.HasVolumes(), "Should have volumes after concurrent additions")
	assert.Greater(t, policy.VolumeCount(), 0, "Should have at least one volume")
}

func TestAutogrowPolicy_ConcurrentMixedOperations(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	var wg sync.WaitGroup

	// Add some initial volumes
	for i := 0; i < 10; i++ {
		policy.AddVolume("volume-" + string(rune('0'+i)))
	}

	// Test concurrent mixed operations (add, remove, read)
	for i := 0; i < 100; i++ {
		wg.Add(3)

		// Concurrent add
		go func(index int) {
			defer wg.Done()
			volumeName := "volume-" + string(rune('0'+index%20))
			policy.AddVolume(volumeName)
		}(i)

		// Concurrent read
		go func(index int) {
			defer wg.Done()
			volumeName := "volume-" + string(rune('0'+index%20))
			_ = policy.HasVolume(volumeName)
			_ = policy.GetVolumes()
			_ = policy.VolumeCount()
		}(i)

		// Concurrent remove
		go func(index int) {
			defer wg.Done()
			volumeName := "volume-" + string(rune('0'+index%20))
			policy.RemoveVolume(volumeName)
		}(i)
	}

	wg.Wait()

	// Just verify no panic occurred and we can still call methods
	_ = policy.VolumeCount()
	_ = policy.HasVolumes()
}

// SmartCopy Tests
func TestAutogrowPolicy_SmartCopy(t *testing.T) {
	tests := map[string]struct {
		policy *AutogrowPolicy
	}{
		"Policy with Success state": {
			policy: NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
		},
		"Policy with Failed state": {
			policy: NewAutogrowPolicy("test-policy-2", "90", "30", "2000", AutogrowPolicyStateFailed),
		},
		"Policy with Deleting state": {
			policy: NewAutogrowPolicy("test-policy-3", "75", "25", "1500", AutogrowPolicyStateDeleting),
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			// Add some volumes to the policy
			test.policy.AddVolume("volume-1")
			test.policy.AddVolume("volume-2")

			// Call SmartCopy
			copied := test.policy.SmartCopy()

			// Assert that SmartCopy returns a deep copy
			copiedPolicy, ok := copied.(*AutogrowPolicy)
			assert.True(t, ok, "SmartCopy should return *AutogrowPolicy")

			// Verify that it's a different pointer (deep copy)
			assert.NotSame(t, test.policy, copiedPolicy, "SmartCopy should return a different pointer (deep copy)")

			// Verify that the copied policy has the same values
			assert.Equal(t, test.policy.Name(), copiedPolicy.Name(), "Names should match")
			assert.Equal(t, test.policy.UsedThreshold(), copiedPolicy.UsedThreshold(), "UsedThreshold should match")
			assert.Equal(t, test.policy.GrowthAmount(), copiedPolicy.GrowthAmount(), "GrowthAmount should match")
			assert.Equal(t, test.policy.MaxSize(), copiedPolicy.MaxSize(), "MaxSize should match")
			assert.Equal(t, test.policy.State(), copiedPolicy.State(), "State should match")

			// Verify that volumes are copied
			assert.True(t, copiedPolicy.HasVolume("volume-1"), "Copied policy should have volume-1")
			assert.True(t, copiedPolicy.HasVolume("volume-2"), "Copied policy should have volume-2")
			assert.Equal(t, 2, copiedPolicy.VolumeCount(), "Copied policy should have same volume count")

			// Verify independence - modifying the copy should not affect the original
			copiedPolicy.AddVolume("volume-3")
			assert.True(t, copiedPolicy.HasVolume("volume-3"), "Copied policy should have volume-3")
			assert.False(t, test.policy.HasVolume("volume-3"), "Original policy should not have volume-3")
			assert.Equal(t, 2, test.policy.VolumeCount(), "Original volume count should remain 2")
		})
	}
}

func TestAutogrowPolicy_SmartCopyInteriorMutability(t *testing.T) {
	// Create a policy
	originalPolicy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	originalPolicy.AddVolume("volume-1")

	// Get a SmartCopy (deep copy)
	copied := originalPolicy.SmartCopy().(*AutogrowPolicy)

	// Verify initial state is copied
	assert.True(t, copied.HasVolume("volume-1"), "Copied policy should have volume-1")
	assert.Equal(t, 1, copied.VolumeCount(), "Copied policy should have 1 volume")

	// Modify the original by adding a volume
	originalPolicy.AddVolume("volume-2")

	// The copy should NOT see the new volume (deep copy, not interior mutability)
	assert.False(t, copied.HasVolume("volume-2"), "SmartCopy creates independent copy")
	assert.Equal(t, 1, copied.VolumeCount(), "Copied volume count should remain 1")
	assert.Equal(t, 2, originalPolicy.VolumeCount(), "Original volume count should be 2")

	// Modify via the copy
	copied.AddVolume("volume-3")

	// Original should NOT see the change (independent copies)
	assert.False(t, originalPolicy.HasVolume("volume-3"), "Original should not see changes to copy")
	assert.Equal(t, 2, originalPolicy.VolumeCount(), "Original volume count should remain 2")
	assert.Equal(t, 2, copied.VolumeCount(), "Copied volume count should be 2")
}

// ConstructExternalWithVolumes Tests

func TestAutogrowPolicy_ConstructExternalWithVolumes(t *testing.T) {
	tests := map[string]struct {
		policy          *AutogrowPolicy
		providedVolumes []string
		expectedCount   int
	}{
		"Policy with empty volume list": {
			policy:          NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			providedVolumes: []string{},
			expectedCount:   0,
		},
		"Policy with single volume": {
			policy:          NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			providedVolumes: []string{"volume-1"},
			expectedCount:   1,
		},
		"Policy with multiple volumes": {
			policy:          NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess),
			providedVolumes: []string{"volume-1", "volume-2", "volume-3"},
			expectedCount:   3,
		},
		"Policy with Failed state and volumes": {
			policy:          NewAutogrowPolicy("test-policy-2", "90", "30", "2000", AutogrowPolicyStateFailed),
			providedVolumes: []string{"volume-1", "volume-2"},
			expectedCount:   2,
		},
		"Policy with Deleting state and volumes": {
			policy:          NewAutogrowPolicy("test-policy-3", "75", "25", "1500", AutogrowPolicyStateDeleting),
			providedVolumes: []string{"volume-1"},
			expectedCount:   1,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			ctx := context.Background()
			external := test.policy.ConstructExternalWithVolumes(ctx, test.providedVolumes)

			assert.NotNil(t, external, "External should not be nil")
			assert.Equal(t, test.policy.Name(), external.Name, "Name mismatch")
			assert.Equal(t, test.policy.UsedThreshold(), external.UsedThreshold, "UsedThreshold mismatch")
			assert.Equal(t, test.policy.GrowthAmount(), external.GrowthAmount, "GrowthAmount mismatch")
			assert.Equal(t, test.policy.MaxSize(), external.MaxSize, "MaxSize mismatch")
			assert.Equal(t, test.policy.State(), external.State, "State mismatch")
			assert.Equal(t, test.expectedCount, external.VolumeCount, "VolumeCount mismatch")
			assert.Len(t, external.Volumes, test.expectedCount, "Volumes length mismatch")

			// Verify the provided volumes are in the external representation
			for _, vol := range test.providedVolumes {
				assert.Contains(t, external.Volumes, vol, "Volume should be in external representation")
			}
		})
	}
}

func TestAutogrowPolicy_ConstructExternalWithVolumes_IgnoresInternalVolumes(t *testing.T) {
	// Create a policy with internal volume associations
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	policy.AddVolume("internal-vol-1")
	policy.AddVolume("internal-vol-2")
	policy.AddVolume("internal-vol-3")

	// Verify internal volumes are there
	assert.Equal(t, 3, policy.VolumeCount(), "Should have 3 internal volumes")

	// Provide a different set of volumes to ConstructExternalWithVolumes
	providedVolumes := []string{"external-vol-1", "external-vol-2"}

	ctx := context.Background()
	external := policy.ConstructExternalWithVolumes(ctx, providedVolumes)

	// The external representation should use the provided volumes, not internal ones
	assert.Equal(t, 2, external.VolumeCount, "VolumeCount should match provided volumes")
	assert.Len(t, external.Volumes, 2, "Volumes length should match provided volumes")
	assert.Contains(t, external.Volumes, "external-vol-1", "Should contain external-vol-1")
	assert.Contains(t, external.Volumes, "external-vol-2", "Should contain external-vol-2")
	assert.NotContains(t, external.Volumes, "internal-vol-1", "Should not contain internal volumes")
	assert.NotContains(t, external.Volumes, "internal-vol-2", "Should not contain internal volumes")
	assert.NotContains(t, external.Volumes, "internal-vol-3", "Should not contain internal volumes")
}

func TestAutogrowPolicy_ConstructExternalWithVolumes_vs_ConstructExternal(t *testing.T) {
	// Create a policy with volumes
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	policy.AddVolume("volume-1")
	policy.AddVolume("volume-2")

	ctx := context.Background()

	// Test ConstructExternal (uses internal volume list)
	externalFromInternal := policy.ConstructExternal(ctx)
	assert.Equal(t, 2, externalFromInternal.VolumeCount, "ConstructExternal should use internal volumes")
	assert.Contains(t, externalFromInternal.Volumes, "volume-1")
	assert.Contains(t, externalFromInternal.Volumes, "volume-2")

	// Test ConstructExternalWithVolumes (uses provided volume list)
	providedVolumes := []string{"volume-3", "volume-4", "volume-5"}
	externalWithProvided := policy.ConstructExternalWithVolumes(ctx, providedVolumes)
	assert.Equal(t, 3, externalWithProvided.VolumeCount, "ConstructExternalWithVolumes should use provided volumes")
	assert.Contains(t, externalWithProvided.Volumes, "volume-3")
	assert.Contains(t, externalWithProvided.Volumes, "volume-4")
	assert.Contains(t, externalWithProvided.Volumes, "volume-5")
	assert.NotContains(t, externalWithProvided.Volumes, "volume-1")
	assert.NotContains(t, externalWithProvided.Volumes, "volume-2")
}

func TestAutogrowPolicy_ConstructExternalWithVolumes_EmptyPolicy(t *testing.T) {
	// Test with a policy that has no internal volumes
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	// Provide an empty volume list
	ctx := context.Background()
	external := policy.ConstructExternalWithVolumes(ctx, []string{})

	assert.NotNil(t, external, "External should not be nil")
	assert.Equal(t, 0, external.VolumeCount, "VolumeCount should be 0")
	assert.Empty(t, external.Volumes, "Volumes should be empty")
	assert.Equal(t, policy.Name(), external.Name, "Name should match")
}

func TestAutogrowPolicy_ConstructExternalWithVolumes_NilVolumes(t *testing.T) {
	// Test with nil volume list
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)

	ctx := context.Background()
	external := policy.ConstructExternalWithVolumes(ctx, nil)

	assert.NotNil(t, external, "External should not be nil")
	assert.Equal(t, 0, external.VolumeCount, "VolumeCount should be 0 for nil slice")
	assert.Nil(t, external.Volumes, "Volumes should be nil")
}

func TestAutogrowPolicy_ConstructExternalWithVolumes_ConcurrentAccess(t *testing.T) {
	policy := NewAutogrowPolicy("test-policy", "80", "20", "1000", AutogrowPolicyStateSuccess)
	var wg sync.WaitGroup

	// Test concurrent calls to ConstructExternalWithVolumes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx := context.Background()
			volumes := []string{"volume-1", "volume-2"}
			external := policy.ConstructExternalWithVolumes(ctx, volumes)
			assert.NotNil(t, external, "External should not be nil")
			assert.Equal(t, 2, external.VolumeCount, "VolumeCount should be 2")
		}(i)
	}

	wg.Wait()
}
