// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTridentAutogrowRequestInternal_GetKind(t *testing.T) {
	tests := []struct {
		name     string
		request  *TridentAutogrowRequestInternal
		expected string
	}{
		{
			name: "Kind from TypeMeta",
			request: &TridentAutogrowRequestInternal{
				TypeMeta: metav1.TypeMeta{
					Kind: "TridentAutogrowRequestInternal",
				},
			},
			expected: "TridentAutogrowRequestInternal",
		},
		{
			name: "Empty Kind falls back to literal",
			request: &TridentAutogrowRequestInternal{
				TypeMeta: metav1.TypeMeta{},
			},
			expected: "TridentAutogrowRequestInternal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.request.GetKind())
		})
	}
}

func TestTridentAutogrowRequestInternal_GetFinalizers(t *testing.T) {
	tests := []struct {
		name     string
		request  *TridentAutogrowRequestInternal
		expected []string
	}{
		{
			name: "Request with finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"finalizer1", "finalizer2"},
				},
			},
			expected: []string{"finalizer1", "finalizer2"},
		},
		{
			name: "Request with empty finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{},
				},
			},
			expected: []string{},
		},
		{
			name: "Request with nil finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: nil,
				},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.request.GetFinalizers()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTridentAutogrowRequestInternal_HasTridentFinalizers(t *testing.T) {
	tridentFinalizers := GetTridentFinalizers()

	tests := []struct {
		name     string
		request  *TridentAutogrowRequestInternal
		expected bool
	}{
		{
			name: "Request with Trident finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tridentFinalizers,
				},
			},
			expected: true,
		},
		{
			name: "Request with partial Trident finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{tridentFinalizers[0], "other-finalizer"},
				},
			},
			expected: true,
		},
		{
			name: "Request without Trident finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other-finalizer"},
				},
			},
			expected: false,
		},
		{
			name: "Request with empty finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{},
				},
			},
			expected: false,
		},
		{
			name: "Request with nil finalizers",
			request: &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: nil,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.request.HasTridentFinalizers()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTridentAutogrowRequestInternal_AddTridentFinalizers(t *testing.T) {
	tridentFinalizers := GetTridentFinalizers()

	tests := []struct {
		name              string
		initialFinalizers []string
		expectedCount     int
	}{
		{
			name:              "Add to empty finalizers",
			initialFinalizers: []string{},
			expectedCount:     len(tridentFinalizers),
		},
		{
			name:              "Add to nil finalizers",
			initialFinalizers: nil,
			expectedCount:     len(tridentFinalizers),
		},
		{
			name:              "Add to existing non-Trident finalizers",
			initialFinalizers: []string{"other-finalizer"},
			expectedCount:     len(tridentFinalizers) + 1,
		},
		{
			name:              "Add when Trident finalizers already exist",
			initialFinalizers: tridentFinalizers,
			expectedCount:     len(tridentFinalizers),
		},
		{
			name:              "Add when some Trident finalizers exist",
			initialFinalizers: []string{tridentFinalizers[0], "other-finalizer"},
			expectedCount:     len(tridentFinalizers) + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.initialFinalizers,
				},
			}

			request.AddTridentFinalizers()

			assert.Equal(t, tt.expectedCount, len(request.ObjectMeta.Finalizers))

			// Verify all Trident finalizers are present
			for _, tf := range tridentFinalizers {
				assert.Contains(t, request.ObjectMeta.Finalizers, tf)
			}

			// Verify no duplicate finalizers
			finalizerMap := make(map[string]int)
			for _, f := range request.ObjectMeta.Finalizers {
				finalizerMap[f]++
			}
			for finalizer, count := range finalizerMap {
				assert.Equal(t, 1, count, "Finalizer %s should appear exactly once", finalizer)
			}
		})
	}
}

func TestTridentAutogrowRequestInternal_RemoveTridentFinalizers(t *testing.T) {
	tridentFinalizers := GetTridentFinalizers()

	tests := []struct {
		name               string
		initialFinalizers  []string
		expectedFinalizers []string
	}{
		{
			name:               "Remove from Trident-only finalizers",
			initialFinalizers:  tridentFinalizers,
			expectedFinalizers: []string{},
		},
		{
			name:               "Remove from mixed finalizers",
			initialFinalizers:  append([]string{"other-finalizer"}, tridentFinalizers...),
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
			request := &TridentAutogrowRequestInternal{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: tt.initialFinalizers,
				},
			}

			request.RemoveTridentFinalizers()

			assert.ElementsMatch(t, tt.expectedFinalizers, request.ObjectMeta.Finalizers)

			// Verify no Trident finalizers remain
			for _, tf := range tridentFinalizers {
				assert.NotContains(t, request.ObjectMeta.Finalizers, tf)
			}
		})
	}
}

func TestTridentAutogrowRequestInternal_FinalizerWorkflow(t *testing.T) {
	// Test a complete workflow: add then remove
	request := &TridentAutogrowRequestInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-request",
			Namespace:  "trident",
			Finalizers: []string{"other-finalizer"},
		},
	}

	// Initially should not have Trident finalizers
	assert.False(t, request.HasTridentFinalizers(), "Should not have Trident finalizers initially")

	// Add Trident finalizers
	request.AddTridentFinalizers()
	assert.True(t, request.HasTridentFinalizers(), "Should have Trident finalizers after adding")
	assert.Contains(t, request.ObjectMeta.Finalizers, "other-finalizer", "Should preserve existing finalizers")

	// Get finalizers should include both
	finalizers := request.GetFinalizers()
	assert.Greater(t, len(finalizers), 1, "Should have multiple finalizers")

	// Remove Trident finalizers
	request.RemoveTridentFinalizers()
	assert.False(t, request.HasTridentFinalizers(), "Should not have Trident finalizers after removal")
	assert.Contains(t, request.ObjectMeta.Finalizers, "other-finalizer", "Should still have other finalizers")
	assert.Equal(t, 1, len(request.ObjectMeta.Finalizers), "Should only have one finalizer left")
}

func TestTridentAutogrowRequestInternal_IdempotentOperations(t *testing.T) {
	tridentFinalizers := GetTridentFinalizers()

	request := &TridentAutogrowRequestInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-request",
			Namespace: "trident",
		},
	}

	// Add multiple times should be idempotent
	request.AddTridentFinalizers()
	firstAddCount := len(request.ObjectMeta.Finalizers)

	request.AddTridentFinalizers()
	secondAddCount := len(request.ObjectMeta.Finalizers)

	assert.Equal(t, firstAddCount, secondAddCount, "Adding finalizers multiple times should be idempotent")
	assert.Equal(t, len(tridentFinalizers), secondAddCount, "Should have exactly the number of Trident finalizers")

	// Remove multiple times should be idempotent
	request.RemoveTridentFinalizers()
	firstRemoveCount := len(request.ObjectMeta.Finalizers)

	request.RemoveTridentFinalizers()
	secondRemoveCount := len(request.ObjectMeta.Finalizers)

	assert.Equal(t, firstRemoveCount, secondRemoveCount, "Removing finalizers multiple times should be idempotent")
	assert.Equal(t, 0, secondRemoveCount, "Should have no finalizers after removal")
}

// TestTridentAutogrowRequestInternalSpec_ObservedUsedBytes verifies the optional ObservedUsedBytes field is stored and preserved (e.g. by DeepCopy).
func TestTridentAutogrowRequestInternalSpec_ObservedUsedBytes(t *testing.T) {
	spec := TridentAutogrowRequestInternalSpec{
		Volume:                "test-pv",
		ObservedCapacityBytes: "5368709120",
		ObservedUsedPercent:   85.0,
		ObservedUsedBytes:     "4563402752", // optional; used bytes at request time
		AutogrowPolicyRef:     TridentAutogrowRequestInternalPolicyRef{Name: "policy", Generation: 1},
	}
	assert.Equal(t, "4563402752", spec.ObservedUsedBytes, "ObservedUsedBytes should be stored")

	// DeepCopy should preserve the field
	copied := spec.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, spec.ObservedUsedBytes, copied.ObservedUsedBytes, "DeepCopy should preserve ObservedUsedBytes")
}
