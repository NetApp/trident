// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/storage"
)

func TestTridentAutogrowPolicy_Persistent(t *testing.T) {
	tests := []struct {
		name     string
		policy   *TridentAutogrowPolicy
		expected *storage.AutogrowPolicyPersistent
	}{
		{
			name: "Complete policy with all fields",
			policy: &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
				Spec: TridentAutogrowPolicySpec{
					UsedThreshold: "80%",
					GrowthAmount:  "20%",
					MaxSize:       "1000Gi",
				},
				Status: TridentAutogrowPolicyStatus{
					State: string(TridentAutogrowPolicyStateSuccess),
				},
			},
			expected: &storage.AutogrowPolicyPersistent{
				Name:          "test-policy",
				UsedThreshold: "80%",
				GrowthAmount:  "20%",
				MaxSize:       "1000Gi",
				State:         storage.AutogrowPolicyStateSuccess,
			},
		},
		{
			name: "Policy with empty MaxSize",
			policy: &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy-no-max",
				},
				Spec: TridentAutogrowPolicySpec{
					UsedThreshold: "90%",
					GrowthAmount:  "10%",
					MaxSize:       "",
				},
				Status: TridentAutogrowPolicyStatus{
					State: string(TridentAutogrowPolicyStateFailed),
				},
			},
			expected: &storage.AutogrowPolicyPersistent{
				Name:          "test-policy-no-max",
				UsedThreshold: "90%",
				GrowthAmount:  "10%",
				MaxSize:       "",
				State:         storage.AutogrowPolicyStateFailed,
			},
		},
		{
			name: "Policy in Deleting state",
			policy: &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy-deleting",
				},
				Spec: TridentAutogrowPolicySpec{
					UsedThreshold: "85%",
					GrowthAmount:  "15%",
					MaxSize:       "2000Gi",
				},
				Status: TridentAutogrowPolicyStatus{
					State: string(TridentAutogrowPolicyStateDeleting),
				},
			},
			expected: &storage.AutogrowPolicyPersistent{
				Name:          "test-policy-deleting",
				UsedThreshold: "85%",
				GrowthAmount:  "15%",
				MaxSize:       "2000Gi",
				State:         storage.AutogrowPolicyStateDeleting,
			},
		},
		{
			name: "Policy with empty GrowthAmount",
			policy: &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy-default-growth",
				},
				Spec: TridentAutogrowPolicySpec{
					UsedThreshold: "75%",
					GrowthAmount:  "",
					MaxSize:       "1500Gi",
				},
				Status: TridentAutogrowPolicyStatus{
					State: string(TridentAutogrowPolicyStateSuccess),
				},
			},
			expected: &storage.AutogrowPolicyPersistent{
				Name:          "test-policy-default-growth",
				UsedThreshold: "75%",
				GrowthAmount:  "",
				MaxSize:       "1500Gi",
				State:         storage.AutogrowPolicyStateSuccess,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persistent, err := tt.policy.Persistent()
			assert.NoError(t, err, "Persistent() should not return error")
			assert.Equal(t, tt.expected, persistent, "Persistent representation should match")
		})
	}
}

func TestTridentAutogrowPolicy_GetObjectMeta(t *testing.T) {
	policy := &TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "",
			UID:       "test-uid-123",
			Labels: map[string]string{
				"app": "test",
			},
		},
	}

	meta := policy.GetObjectMeta()
	assert.Equal(t, "test-policy", meta.Name)
	assert.Equal(t, "test-uid-123", string(meta.UID))
	assert.Equal(t, "test", meta.Labels["app"])
}

func TestTridentAutogrowPolicy_GetKind(t *testing.T) {
	policy := &TridentAutogrowPolicy{}
	assert.Equal(t, "TridentAutogrowPolicy", policy.GetKind())
}

func TestTridentAutogrowPolicy_GetFinalizers(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		expected   []string
	}{
		{
			name:       "With finalizers",
			finalizers: []string{"trident.netapp.io", "other-finalizer"},
			expected:   []string{"trident.netapp.io", "other-finalizer"},
		},
		{
			name:       "Empty finalizers",
			finalizers: []string{},
			expected:   []string{},
		},
		{
			name:       "Nil finalizers",
			finalizers: nil,
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.finalizers,
				},
			}
			result := policy.GetFinalizers()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTridentAutogrowPolicy_HasTridentFinalizers(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		expected   bool
	}{
		{
			name:       "Has Trident finalizer",
			finalizers: []string{TridentFinalizerName},
			expected:   true,
		},
		{
			name:       "Has Trident finalizer with others",
			finalizers: []string{"other-finalizer", TridentFinalizerName, "another-finalizer"},
			expected:   true,
		},
		{
			name:       "No Trident finalizer",
			finalizers: []string{"other-finalizer"},
			expected:   false,
		},
		{
			name:       "Empty finalizers",
			finalizers: []string{},
			expected:   false,
		},
		{
			name:       "Nil finalizers",
			finalizers: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.finalizers,
				},
			}
			result := policy.HasTridentFinalizers()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTridentAutogrowPolicy_AddTridentFinalizers(t *testing.T) {
	tests := []struct {
		name               string
		initialFinalizers  []string
		expectedFinalizers []string
	}{
		{
			name:               "Add to nil finalizers",
			initialFinalizers:  nil,
			expectedFinalizers: GetTridentFinalizers(),
		},
		{
			name:               "Add to empty finalizers",
			initialFinalizers:  []string{},
			expectedFinalizers: GetTridentFinalizers(),
		},
		{
			name:               "Add to existing non-Trident finalizers",
			initialFinalizers:  []string{"other-finalizer"},
			expectedFinalizers: append([]string{"other-finalizer"}, GetTridentFinalizers()...),
		},
		{
			name:               "Already has Trident finalizers",
			initialFinalizers:  GetTridentFinalizers(),
			expectedFinalizers: GetTridentFinalizers(),
		},
		{
			name:               "Has Trident finalizers plus others",
			initialFinalizers:  append([]string{"other-finalizer"}, GetTridentFinalizers()...),
			expectedFinalizers: append([]string{"other-finalizer"}, GetTridentFinalizers()...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.initialFinalizers,
				},
			}
			policy.AddTridentFinalizers()

			// Verify all Trident finalizers are present
			for _, expectedFinalizer := range GetTridentFinalizers() {
				assert.Contains(t, policy.ObjectMeta.Finalizers, expectedFinalizer)
			}

			// Verify no duplicates were added
			assert.Len(t, policy.ObjectMeta.Finalizers, len(tt.expectedFinalizers))
		})
	}
}

func TestTridentAutogrowPolicy_RemoveTridentFinalizers(t *testing.T) {
	tests := []struct {
		name               string
		initialFinalizers  []string
		expectedFinalizers []string
	}{
		{
			name:               "Remove from Trident finalizers only",
			initialFinalizers:  GetTridentFinalizers(),
			expectedFinalizers: []string{},
		},
		{
			name:               "Remove from mixed finalizers",
			initialFinalizers:  append([]string{"other-finalizer"}, GetTridentFinalizers()...),
			expectedFinalizers: []string{"other-finalizer"},
		},
		{
			name:               "Remove from empty finalizers",
			initialFinalizers:  []string{},
			expectedFinalizers: []string{},
		},
		{
			name:               "Remove from nil finalizers",
			initialFinalizers:  nil,
			expectedFinalizers: []string{},
		},
		{
			name:               "Remove when no Trident finalizers present",
			initialFinalizers:  []string{"other-finalizer", "another-finalizer"},
			expectedFinalizers: []string{"other-finalizer", "another-finalizer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.initialFinalizers,
				},
			}
			policy.RemoveTridentFinalizers()

			// Verify all Trident finalizers are removed
			for _, tridentFinalizer := range GetTridentFinalizers() {
				assert.NotContains(t, policy.ObjectMeta.Finalizers, tridentFinalizer)
			}

			// Verify expected finalizers remain
			assert.ElementsMatch(t, tt.expectedFinalizers, policy.ObjectMeta.Finalizers)
		})
	}
}

func TestTridentAutogrowPolicy_AddAndRemoveFinalizers(t *testing.T) {
	policy := &TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy",
			Finalizers: []string{"other-finalizer"},
		},
	}

	// Initially should not have Trident finalizers
	assert.False(t, policy.HasTridentFinalizers())

	// Add Trident finalizers
	policy.AddTridentFinalizers()
	assert.True(t, policy.HasTridentFinalizers())
	assert.Contains(t, policy.ObjectMeta.Finalizers, "other-finalizer")

	// Add again - should not create duplicates
	policy.AddTridentFinalizers()
	assert.True(t, policy.HasTridentFinalizers())

	// Remove Trident finalizers
	policy.RemoveTridentFinalizers()
	assert.False(t, policy.HasTridentFinalizers())
	assert.Contains(t, policy.ObjectMeta.Finalizers, "other-finalizer")
	assert.NotContains(t, policy.ObjectMeta.Finalizers, TridentFinalizerName)
}

func TestTridentAutogrowPolicy_RoundTripPersistent(t *testing.T) {
	original := &TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roundtrip-policy",
		},
		Spec: TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			GrowthAmount:  "20%",
			MaxSize:       "1000Gi",
		},
		Status: TridentAutogrowPolicyStatus{
			State:   string(TridentAutogrowPolicyStateSuccess),
			Message: "Policy validated and ready",
		},
	}

	// Convert to persistent
	persistent, err := original.Persistent()
	assert.NoError(t, err)
	assert.NotNil(t, persistent)

	// Verify all fields match
	assert.Equal(t, original.ObjectMeta.Name, persistent.Name)
	assert.Equal(t, original.Spec.UsedThreshold, persistent.UsedThreshold)
	assert.Equal(t, original.Spec.GrowthAmount, persistent.GrowthAmount)
	assert.Equal(t, original.Spec.MaxSize, persistent.MaxSize)
	assert.Equal(t, storage.AutogrowPolicyState(original.Status.State), persistent.State)
}

func TestTridentAutogrowPolicyState_Constants(t *testing.T) {
	tests := []struct {
		name     string
		state    TridentAutogrowPolicyState
		expected string
	}{
		{
			name:     "Success state",
			state:    TridentAutogrowPolicyStateSuccess,
			expected: "Success",
		},
		{
			name:     "Failed state",
			state:    TridentAutogrowPolicyStateFailed,
			expected: "Failed",
		},
		{
			name:     "Deleting state",
			state:    TridentAutogrowPolicyStateDeleting,
			expected: "Deleting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.state))
		})
	}
}

func TestTridentAutogrowPolicy_MultipleFinalizerOperations(t *testing.T) {
	policy := &TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-multi-ops",
		},
	}

	// Start with no finalizers
	assert.Empty(t, policy.GetFinalizers())
	assert.False(t, policy.HasTridentFinalizers())

	// Add Trident finalizers
	policy.AddTridentFinalizers()
	assert.NotEmpty(t, policy.GetFinalizers())
	assert.True(t, policy.HasTridentFinalizers())

	// Add more non-Trident finalizers
	policy.ObjectMeta.Finalizers = append(policy.ObjectMeta.Finalizers, "custom-finalizer-1", "custom-finalizer-2")
	assert.True(t, policy.HasTridentFinalizers())
	assert.Contains(t, policy.GetFinalizers(), "custom-finalizer-1")
	assert.Contains(t, policy.GetFinalizers(), "custom-finalizer-2")

	// Remove Trident finalizers
	policy.RemoveTridentFinalizers()
	assert.False(t, policy.HasTridentFinalizers())
	assert.Contains(t, policy.GetFinalizers(), "custom-finalizer-1")
	assert.Contains(t, policy.GetFinalizers(), "custom-finalizer-2")

	// Verify Trident finalizers are gone
	for _, tridentFinalizer := range GetTridentFinalizers() {
		assert.NotContains(t, policy.GetFinalizers(), tridentFinalizer)
	}
}

func TestTridentAutogrowPolicy_EmptyPolicy(t *testing.T) {
	policy := &TridentAutogrowPolicy{}

	// Test Persistent with minimal policy
	persistent, err := policy.Persistent()
	assert.NoError(t, err)
	assert.NotNil(t, persistent)
	assert.Empty(t, persistent.Name)
	assert.Empty(t, persistent.UsedThreshold)
	assert.Empty(t, persistent.GrowthAmount)
	assert.Empty(t, persistent.MaxSize)
	assert.Empty(t, persistent.State)

	// Test other methods
	assert.Empty(t, policy.GetObjectMeta().Name)
	assert.Equal(t, "TridentAutogrowPolicy", policy.GetKind())
	assert.Empty(t, policy.GetFinalizers())
	assert.False(t, policy.HasTridentFinalizers())
}
