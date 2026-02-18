package concurrent_cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

func TestListSubordinateVolumes(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		expected           int
	}{
		{
			name:               "no subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			expected:           0,
		},
		{
			name: "single subordinate volume",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{Name: "sub-vol-1"},
					State:  storage.VolumeStateOnline,
				},
			},
			expected: 1,
		},
		{
			name: "multiple subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{Name: "sub-vol-1"},
					State:  storage.VolumeStateOnline,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{Name: "sub-vol-2"},
					State:  storage.VolumeStateOnline,
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumes
			subquery := ListSubordinateVolumes()
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSubordinateVolumes setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, "Number of subordinate volumes should match expected")

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestListSubordinateVolumesForVolume(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		volumeID           string
		expected           int
	}{
		{
			name:               "no matching subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			volumeID:           "nonexistent-volume",
			expected:           0,
		},
		{
			name: "single matching subordinate volume",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:              "sub-vol-1",
						ShareSourceVolume: "target-volume",
						VolumeMode:        config.Filesystem,
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{
						Name:              "sub-vol-2",
						ShareSourceVolume: "other-volume",
						VolumeMode:        config.Filesystem,
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			volumeID: "target-volume",
			expected: 1,
		},
		{
			name: "multiple matching subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:              "sub-vol-1",
						ShareSourceVolume: "target-volume",
						VolumeMode:        config.Filesystem,
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{
						Name:              "sub-vol-2",
						ShareSourceVolume: "target-volume",
						VolumeMode:        config.Filesystem,
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			volumeID: "target-volume",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumesForVolume
			subquery := ListSubordinateVolumesForVolume(tt.volumeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSubordinateVolumesForVolume setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, "Number of subordinate volumes should match expected")
			for _, sv := range result.SubordinateVolumes {
				assert.Equal(t, tt.volumeID, sv.Config.ShareSourceVolume, "Subordinate volume source should match filter")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestReadSubordinateVolume(t *testing.T) {
	tests := []struct {
		name                      string
		setupSubordinateVolume    bool
		subordinateVolumeID       string
		expectedSubordinateVolume *storage.Volume
	}{
		{
			name:                   "existing subordinate volume",
			setupSubordinateVolume: true,
			subordinateVolumeID:    "test-subordinate-volume",
			expectedSubordinateVolume: &storage.Volume{
				Config: &storage.VolumeConfig{Name: "test-subordinate-volume"},
				State:  storage.VolumeStateOnline,
			},
		},
		{
			name:                   "non-existing subordinate volume",
			setupSubordinateVolume: false,
			subordinateVolumeID:    "non-existing-subordinate-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			if tt.setupSubordinateVolume {
				subordinateVolumes.data[tt.subordinateVolumeID] = tt.expectedSubordinateVolume
			}
			subordinateVolumes.unlock()

			// Execute ReadSubordinateVolume
			subquery := ReadSubordinateVolume(tt.subordinateVolumeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadSubordinateVolume setResults should not error")

			// Verify results
			if tt.setupSubordinateVolume {
				assert.NotNil(t, result.SubordinateVolume.Read, "SubordinateVolume should be found")
				assert.Equal(t, tt.expectedSubordinateVolume.Config.Name, result.SubordinateVolume.Read.Config.Name, "SubordinateVolume name should match expected")
			} else {
				assert.Nil(t, result.SubordinateVolume.Read, "SubordinateVolume should not be found")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestInconsistentReadSubordinateVolume(t *testing.T) {
	tests := []struct {
		name                      string
		setupSubordinateVolume    bool
		subordinateVolumeID       string
		expectedSubordinateVolume *storage.Volume
	}{
		{
			name:                   "existing subordinate volume",
			setupSubordinateVolume: true,
			subordinateVolumeID:    "test-subordinate-volume",
			expectedSubordinateVolume: &storage.Volume{
				Config: &storage.VolumeConfig{Name: "test-subordinate-volume"},
				State:  storage.VolumeStateOnline,
			},
		},
		{
			name:                   "non-existing subordinate volume",
			setupSubordinateVolume: false,
			subordinateVolumeID:    "non-existing-subordinate-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			if tt.setupSubordinateVolume {
				subordinateVolumes.data[tt.subordinateVolumeID] = tt.expectedSubordinateVolume
			}
			subordinateVolumes.unlock()

			// Execute InconsistentReadSubordinateVolume
			subquery := InconsistentReadSubordinateVolume(tt.subordinateVolumeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadSubordinateVolume setResults should not error")

			// Verify results
			if tt.setupSubordinateVolume {
				assert.NotNil(t, result.SubordinateVolume.Read, "SubordinateVolume should be found")
				assert.Equal(t, tt.expectedSubordinateVolume.Config.Name, result.SubordinateVolume.Read.Config.Name, "SubordinateVolume name should match expected")
			} else {
				assert.Nil(t, result.SubordinateVolume.Read, "SubordinateVolume should not be found")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestUpsertSubordinateVolume(t *testing.T) {
	tests := []struct {
		name                      string
		setupSubordinateVolume    bool
		subordinateVolumeID       string
		volumeID                  string
		expectedSubordinateVolume *storage.Volume
	}{
		{
			name:                   "existing subordinate volume",
			setupSubordinateVolume: true,
			subordinateVolumeID:    "test-subordinate-volume",
			volumeID:               "parent-volume",
			expectedSubordinateVolume: &storage.Volume{
				Config: &storage.VolumeConfig{Name: "test-subordinate-volume"},
				State:  storage.VolumeStateOnline,
			},
		},
		{
			name:                   "non-existing subordinate volume",
			setupSubordinateVolume: false,
			subordinateVolumeID:    "new-subordinate-volume",
			volumeID:               "parent-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			if tt.setupSubordinateVolume {
				subordinateVolumes.data[tt.subordinateVolumeID] = tt.expectedSubordinateVolume
			}
			subordinateVolumes.unlock()

			// Execute UpsertSubordinateVolume
			subquery := UpsertSubordinateVolume(tt.subordinateVolumeID, tt.volumeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertSubordinateVolume setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.SubordinateVolume.Upsert, "Upsert function should be created")

			if tt.setupSubordinateVolume {
				assert.NotNil(t, result.SubordinateVolume.Read, "SubordinateVolume should be read")
			}

			// Test the upsert function with a new volume
			newVolume := &storage.Volume{
				Config: &storage.VolumeConfig{Name: "upserted-subordinate-volume"},
				State:  storage.VolumeStateOnline,
			}
			result.SubordinateVolume.Upsert(newVolume)

			// Verify the volume was actually added/updated in storage
			subordinateVolumes.rlock()
			stored, exists := subordinateVolumes.data[tt.subordinateVolumeID]
			subordinateVolumes.runlock()
			assert.True(t, exists, "SubordinateVolume should exist in storage after upsert")
			assert.Equal(t, newVolume, stored, "SubordinateVolume should match upserted volume")

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestDeleteSubordinateVolume(t *testing.T) {
	tests := []struct {
		name                      string
		setupSubordinateVolume    bool
		subordinateVolumeID       string
		expectedSubordinateVolume *storage.Volume
	}{
		{
			name:                   "existing subordinate volume",
			setupSubordinateVolume: true,
			subordinateVolumeID:    "test-subordinate-volume",
			expectedSubordinateVolume: &storage.Volume{
				Config: &storage.VolumeConfig{Name: "test-subordinate-volume"},
				State:  storage.VolumeStateOnline,
			},
		},
		{
			name:                   "non-existing subordinate volume",
			setupSubordinateVolume: false,
			subordinateVolumeID:    "non-existing-subordinate-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			if tt.setupSubordinateVolume {
				subordinateVolumes.data[tt.subordinateVolumeID] = tt.expectedSubordinateVolume
			}
			subordinateVolumes.unlock()

			// Execute DeleteSubordinateVolume
			subquery := DeleteSubordinateVolume(tt.subordinateVolumeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteSubordinateVolume setResults should not error")

			// Verify the delete function was created
			assert.NotNil(t, result.SubordinateVolume.Delete, "Delete function should be created")

			if tt.setupSubordinateVolume {
				assert.NotNil(t, result.SubordinateVolume.Read, "SubordinateVolume should be read")
				assert.Equal(t, tt.expectedSubordinateVolume.Config.Name, result.SubordinateVolume.Read.Config.Name, "SubordinateVolume name should match expected")

				// Call the delete function
				result.SubordinateVolume.Delete()

				// Verify the subordinate volume was actually removed from storage
				subordinateVolumes.rlock()
				_, exists := subordinateVolumes.data[tt.subordinateVolumeID]
				subordinateVolumes.runlock()
				assert.False(t, exists, "SubordinateVolume should not exist in storage after delete")
			} else {
				assert.Nil(t, result.SubordinateVolume.Read, "SubordinateVolume should not be read for non-existing")

				// Call the delete function (should not crash)
				result.SubordinateVolume.Delete()
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestListSubordinateVolumesForAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		policyName         string
		expected           int
	}{
		{
			name:               "no subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			policyName:         "policy1",
			expected:           0,
		},
		{
			name: "single matching subordinate volume",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{Name: "sub-vol-1"},
					State:  storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
					},
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{Name: "sub-vol-2"},
					State:  storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy2",
					},
				},
			},
			policyName: "policy1",
			expected:   1,
		},
		{
			name: "multiple matching subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{Name: "sub-vol-1"},
					State:  storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
					},
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{Name: "sub-vol-2"},
					State:  storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
					},
				},
			},
			policyName: "policy1",
			expected:   2,
		},
		{
			name: "no matching policy",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{Name: "sub-vol-1"},
					State:  storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy2",
					},
				},
			},
			policyName: "policy1",
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumesForAutogrowPolicy
			subquery := ListSubordinateVolumesForAutogrowPolicy(tt.policyName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSubordinateVolumesForAutogrowPolicy setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, "Number of subordinate volumes should match expected")
			for _, sv := range result.SubordinateVolumes {
				assert.Equal(t, tt.policyName, sv.EffectiveAGPolicy.PolicyName,
					"Subordinate volume effective policy should match filter")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestListSubordinateVolumesForStorageClass(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		scName             string
		expected           int
	}{
		{
			name:               "no subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			scName:             "sc1",
			expected:           0,
		},
		{
			name: "single matching subordinate volume",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:         "sub-vol-1",
						StorageClass: "sc1",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{
						Name:         "sub-vol-2",
						StorageClass: "sc2",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 1,
		},
		{
			name: "multiple matching subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:         "sub-vol-1",
						StorageClass: "sc1",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{
						Name:         "sub-vol-2",
						StorageClass: "sc1",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 2,
		},
		{
			name: "no matching storage class",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:         "sub-vol-1",
						StorageClass: "sc2",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumesForStorageClass
			subquery := ListSubordinateVolumesForStorageClass(tt.scName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSubordinateVolumesForStorageClass setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, "Number of subordinate volumes should match expected")
			for _, sv := range result.SubordinateVolumes {
				assert.Equal(t, tt.scName, sv.Config.StorageClass,
					"Subordinate volume storage class should match filter")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestListSubordinateVolumesForStorageClassWithoutAutogrowOverride(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		scName             string
		expected           int
	}{
		{
			name:               "no subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			scName:             "sc1",
			expected:           0,
		},
		{
			name: "matching storage class with no autogrow override",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 1,
		},
		{
			name: "matching storage class with autogrow override - excluded",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "my-policy",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 0,
		},
		{
			name: "different storage class without autogrow override - excluded",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						StorageClass:            "sc2",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 0,
		},
		{
			name: "mixed - some with override some without",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-2": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-2",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "my-policy",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-3": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-3",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			scName:   "sc1",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumesForStorageClassWithoutAutogrowOverride
			subquery := ListSubordinateVolumesForStorageClassWithoutAutogrowOverride(tt.scName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err,
				"ListSubordinateVolumesForStorageClassWithoutAutogrowOverride setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, "Number of subordinate volumes should match expected")
			for _, sv := range result.SubordinateVolumes {
				assert.Equal(t, tt.scName, sv.Config.StorageClass,
					"Subordinate volume storage class should match filter")
				assert.Empty(t, sv.Config.RequestedAutogrowPolicy,
					"Subordinate volume should have no autogrow override")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}

func TestListSubordinateVolumesForAutogrowPolicyReevaluation(t *testing.T) {
	tests := []struct {
		name               string
		subordinateVolumes map[string]*storage.Volume
		policyName         string
		expected           int
		description        string
	}{
		{
			name:               "no subordinate volumes",
			subordinateVolumes: map[string]*storage.Volume{},
			policyName:         "policy1",
			expected:           0,
			description:        "empty cache returns nothing",
		},
		{
			name: "skip - requestedAutogrowPolicy is none (case-insensitive)",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						RequestedAutogrowPolicy: "None",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			policyName:  "policy1",
			expected:    0,
			description: "volumes with requestedAutogrowPolicy='none' are skipped (case-insensitive)",
		},
		{
			name: "skip - requestedAutogrowPolicy references a different policy",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						RequestedAutogrowPolicy: "other-policy",
					},
					State: storage.VolumeStateSubordinate,
				},
			},
			policyName:  "policy1",
			expected:    0,
			description: "volumes explicitly requesting a different policy are skipped",
		},
		{
			name: "skip - already using this policy",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						RequestedAutogrowPolicy: "policy1",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
					},
				},
			},
			policyName:  "policy1",
			expected:    0,
			description: "volumes already using the policy are skipped",
		},
		{
			name: "include - no requestedAutogrowPolicy and not using the policy",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "other-policy",
					},
				},
			},
			policyName:  "policy1",
			expected:    1,
			description: "volumes with no requested policy that might inherit are included",
		},
		{
			name: "include - requestedAutogrowPolicy matches (case-insensitive) and not currently using it",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-1": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-1",
						RequestedAutogrowPolicy: "Policy1",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "old-policy",
					},
				},
			},
			policyName:  "policy1",
			expected:    1,
			description: "volumes requesting the same policy (case-insensitive) but not using it are included",
		},
		{
			name: "mixed - various cases together",
			subordinateVolumes: map[string]*storage.Volume{
				"sub-vol-skip-none": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-skip-none",
						RequestedAutogrowPolicy: "none",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-skip-different": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-skip-different",
						RequestedAutogrowPolicy: "other-policy",
					},
					State: storage.VolumeStateSubordinate,
				},
				"sub-vol-skip-already-using": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-skip-already-using",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
					},
				},
				"sub-vol-include-no-requested": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-include-no-requested",
						RequestedAutogrowPolicy: "",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "old-policy",
					},
				},
				"sub-vol-include-matching": {
					Config: &storage.VolumeConfig{
						Name:                    "sub-vol-include-matching",
						RequestedAutogrowPolicy: "POLICY1",
					},
					State: storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "old-policy",
					},
				},
			},
			policyName:  "policy1",
			expected:    2,
			description: "only volumes eligible for reevaluation are included",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			for k, v := range tt.subordinateVolumes {
				subordinateVolumes.data[k] = v
			}
			subordinateVolumes.unlock()

			// Execute ListSubordinateVolumesForAutogrowPolicyReevaluation
			subquery := ListSubordinateVolumesForAutogrowPolicyReevaluation(tt.policyName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err,
				"ListSubordinateVolumesForAutogrowPolicyReevaluation setResults should not error")

			// Verify results
			assert.Len(t, result.SubordinateVolumes, tt.expected, tt.description)

			// Verify none of the returned volumes already use the policy
			for _, sv := range result.SubordinateVolumes {
				assert.NotEqual(t, tt.policyName, sv.EffectiveAGPolicy.PolicyName,
					"Returned volumes should not already be using the target policy")
			}

			// Clean up
			subordinateVolumes.lock()
			subordinateVolumes.data = make(map[string]SmartCopier)
			subordinateVolumes.unlock()
		})
	}
}
