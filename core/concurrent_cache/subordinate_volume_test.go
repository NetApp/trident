package concurrent_cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
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
