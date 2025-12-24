// Copyright 2024 NetApp, Inc. All Rights Reserved.

package configurator

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRemoveFinalizerFromObjectMeta(t *testing.T) {
	tests := []struct {
		name             string
		objMeta          *metav1.ObjectMeta
		finalizer        string
		expectedRemoved  bool
		expectedCount    int
		expectedContains []string
	}{
		{
			name: "Remove existing finalizer",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2", "finalizer3"},
			},
			finalizer:        "finalizer2",
			expectedRemoved:  true,
			expectedCount:    2,
			expectedContains: []string{"finalizer1", "finalizer3"},
		},
		{
			name: "Remove non-existing finalizer",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2"},
			},
			finalizer:        "finalizer3",
			expectedRemoved:  false,
			expectedCount:    2,
			expectedContains: []string{"finalizer1", "finalizer2"},
		},
		{
			name: "Remove from empty finalizers",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{},
			},
			finalizer:        "finalizer1",
			expectedRemoved:  false,
			expectedCount:    0,
			expectedContains: []string{},
		},
		{
			name:            "Remove from nil ObjectMeta",
			objMeta:         nil,
			finalizer:       "finalizer1",
			expectedRemoved: false,
		},
		{
			name: "Remove only finalizer",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1"},
			},
			finalizer:        "finalizer1",
			expectedRemoved:  true,
			expectedCount:    0,
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removed := RemoveFinalizerFromObjectMeta(tt.objMeta, tt.finalizer)
			if removed != tt.expectedRemoved {
				t.Errorf("RemoveFinalizerFromObjectMeta() returned %v, expected %v", removed, tt.expectedRemoved)
			}

			if tt.objMeta != nil {
				if len(tt.objMeta.Finalizers) != tt.expectedCount {
					t.Errorf("Expected %d finalizers, got %d", tt.expectedCount, len(tt.objMeta.Finalizers))
				}

				for _, expected := range tt.expectedContains {
					found := false
					for _, f := range tt.objMeta.Finalizers {
						if f == expected {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected finalizer %s not found in finalizers", expected)
					}
				}
			}
		})
	}
}

func TestAddFinalizerToObjectMeta(t *testing.T) {
	tests := []struct {
		name             string
		objMeta          *metav1.ObjectMeta
		finalizer        string
		expectedAdded    bool
		expectedCount    int
		expectedContains []string
	}{
		{
			name: "Add new finalizer",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2"},
			},
			finalizer:        "finalizer3",
			expectedAdded:    true,
			expectedCount:    3,
			expectedContains: []string{"finalizer1", "finalizer2", "finalizer3"},
		},
		{
			name: "Add existing finalizer",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2"},
			},
			finalizer:        "finalizer1",
			expectedAdded:    false,
			expectedCount:    2,
			expectedContains: []string{"finalizer1", "finalizer2"},
		},
		{
			name: "Add to empty finalizers",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{},
			},
			finalizer:        "finalizer1",
			expectedAdded:    true,
			expectedCount:    1,
			expectedContains: []string{"finalizer1"},
		},
		{
			name:          "Add to nil ObjectMeta",
			objMeta:       nil,
			finalizer:     "finalizer1",
			expectedAdded: false,
		},
		{
			name:             "Add to nil finalizers slice",
			objMeta:          &metav1.ObjectMeta{},
			finalizer:        "finalizer1",
			expectedAdded:    true,
			expectedCount:    1,
			expectedContains: []string{"finalizer1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added := AddFinalizerToObjectMeta(tt.objMeta, tt.finalizer)
			if added != tt.expectedAdded {
				t.Errorf("AddFinalizerToObjectMeta() returned %v, expected %v", added, tt.expectedAdded)
			}

			if tt.objMeta != nil {
				if len(tt.objMeta.Finalizers) != tt.expectedCount {
					t.Errorf("Expected %d finalizers, got %d", tt.expectedCount, len(tt.objMeta.Finalizers))
				}

				for _, expected := range tt.expectedContains {
					found := false
					for _, f := range tt.objMeta.Finalizers {
						if f == expected {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected finalizer %s not found in finalizers", expected)
					}
				}
			}
		})
	}
}

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name      string
		objMeta   *metav1.ObjectMeta
		finalizer string
		expected  bool
	}{
		{
			name: "Finalizer exists",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2", "finalizer3"},
			},
			finalizer: "finalizer2",
			expected:  true,
		},
		{
			name: "Finalizer does not exist",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{"finalizer1", "finalizer2"},
			},
			finalizer: "finalizer3",
			expected:  false,
		},
		{
			name: "Empty finalizers",
			objMeta: &metav1.ObjectMeta{
				Finalizers: []string{},
			},
			finalizer: "finalizer1",
			expected:  false,
		},
		{
			name:      "Nil ObjectMeta",
			objMeta:   nil,
			finalizer: "finalizer1",
			expected:  false,
		},
		{
			name:      "Nil finalizers slice",
			objMeta:   &metav1.ObjectMeta{},
			finalizer: "finalizer1",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasFinalizer(tt.objMeta, tt.finalizer)
			if result != tt.expected {
				t.Errorf("HasFinalizer() returned %v, expected %v", result, tt.expected)
			}
		})
	}
}
