package concurrent_cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

func TestListVolumePublicationsForVolume(t *testing.T) {
	tests := []struct {
		name               string
		volumePublications map[string]*models.VolumePublication
		volumeName         string
		expected           int
	}{
		{
			name:               "no matching volume publications",
			volumePublications: map[string]*models.VolumePublication{},
			volumeName:         "nonexistent-volume",
			expected:           0,
		},
		{
			name: "single matching volume publication",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "node1",
					VolumeName: "target-volume",
				},
				"vol2.node1": {
					NodeName:   "node1",
					VolumeName: "other-volume",
				},
			},
			volumeName: "target-volume",
			expected:   1,
		},
		{
			name: "multiple matching volume publications",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "node1",
					VolumeName: "target-volume",
				},
				"vol1.node2": {
					NodeName:   "node2",
					VolumeName: "target-volume",
				},
			},
			volumeName: "target-volume",
			expected:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			for k, v := range tt.volumePublications {
				volumePublications.data[k] = v
			}
			volumePublications.unlock()

			// Execute ListVolumePublicationsForVolume
			subquery := ListVolumePublicationsForVolume(tt.volumeName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListVolumePublicationsForVolume setResults should not error")

			// Verify results
			assert.Len(t, result.VolumePublications, tt.expected, "Number of volume publications should match expected")
			for _, vp := range result.VolumePublications {
				assert.Equal(t, tt.volumeName, vp.VolumeName, "Volume publication volume name should match filter")
			}

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}

func TestListVolumePublicationsForNode(t *testing.T) {
	tests := []struct {
		name               string
		volumePublications map[string]*models.VolumePublication
		nodeName           string
		expected           int
	}{
		{
			name:               "no matching volume publications",
			volumePublications: map[string]*models.VolumePublication{},
			nodeName:           "nonexistent-node",
			expected:           0,
		},
		{
			name: "single matching volume publication",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "target-node",
					VolumeName: "vol1",
				},
				"vol1.node2": {
					NodeName:   "other-node",
					VolumeName: "vol1",
				},
			},
			nodeName: "target-node",
			expected: 1,
		},
		{
			name: "multiple matching volume publications",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "target-node",
					VolumeName: "vol1",
				},
				"vol2.node1": {
					NodeName:   "target-node",
					VolumeName: "vol2",
				},
			},
			nodeName: "target-node",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			for k, v := range tt.volumePublications {
				volumePublications.data[k] = v
			}
			volumePublications.unlock()

			// Execute ListVolumePublicationsForNode
			subquery := ListVolumePublicationsForNode(tt.nodeName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListVolumePublicationsForNode setResults should not error")

			// Verify results
			assert.Len(t, result.VolumePublications, tt.expected, "Number of volume publications should match expected")
			for _, vp := range result.VolumePublications {
				assert.Equal(t, tt.nodeName, vp.NodeName, "Volume publication node name should match filter")
			}

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}

func TestInconsistentReadVolumePublication(t *testing.T) {
	tests := []struct {
		name                      string
		setupVolumePublication    bool
		volumeID                  string
		nodeID                    string
		expectedVolumePublication *models.VolumePublication
	}{
		{
			name:                   "existing volume publication",
			setupVolumePublication: true,
			volumeID:               "test-volume",
			nodeID:                 "test-node",
			expectedVolumePublication: &models.VolumePublication{
				NodeName:   "test-node",
				VolumeName: "test-volume",
			},
		},
		{
			name:                   "non-existing volume publication",
			setupVolumePublication: false,
			volumeID:               "non-existing-volume",
			nodeID:                 "non-existing-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			if tt.setupVolumePublication {
				volumePublications.data[tt.volumeID+"."+tt.nodeID] = tt.expectedVolumePublication
			}
			volumePublications.unlock()

			// Execute InconsistentReadVolumePublication
			subquery := InconsistentReadVolumePublication(tt.volumeID, tt.nodeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadVolumePublication setResults should not error")

			// Verify results
			if tt.setupVolumePublication {
				assert.NotNil(t, result.VolumePublication.Read, "VolumePublication should be found")
				assert.Equal(t, tt.expectedVolumePublication, result.VolumePublication.Read, "VolumePublication should match expected")
			} else {
				assert.Nil(t, result.VolumePublication.Read, "VolumePublication should not be found")
			}

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}

func TestDeleteVolumePublication(t *testing.T) {
	tests := []struct {
		name                      string
		setupVolumePublication    bool
		volumeID                  string
		nodeID                    string
		expectedVolumePublication *models.VolumePublication
	}{
		{
			name:                   "existing volume publication",
			setupVolumePublication: true,
			volumeID:               "test-volume",
			nodeID:                 "test-node",
			expectedVolumePublication: &models.VolumePublication{
				NodeName:   "test-node",
				VolumeName: "test-volume",
			},
		},
		{
			name:                   "non-existing volume publication",
			setupVolumePublication: false,
			volumeID:               "non-existing-volume",
			nodeID:                 "non-existing-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			if tt.setupVolumePublication {
				volumePublications.data[tt.volumeID+"."+tt.nodeID] = tt.expectedVolumePublication
			}
			volumePublications.unlock()

			// Execute DeleteVolumePublication
			subquery := DeleteVolumePublication(tt.volumeID, tt.nodeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteVolumePublication setResults should not error")

			// Verify the delete function was created
			assert.NotNil(t, result.VolumePublication.Delete, "Delete function should be created")

			if tt.setupVolumePublication {
				assert.NotNil(t, result.VolumePublication.Read, "VolumePublication should be read")
				assert.Equal(t, tt.expectedVolumePublication, result.VolumePublication.Read, "VolumePublication should match expected")

				// Call the delete function
				result.VolumePublication.Delete()

				// Verify the volume publication was actually removed from storage
				volumePublications.rlock()
				_, exists := volumePublications.data[tt.volumeID+"."+tt.nodeID]
				volumePublications.runlock()
				assert.False(t, exists, "VolumePublication should not exist in storage after delete")
			} else {
				assert.Nil(t, result.VolumePublication.Read, "VolumePublication should not be read for non-existing")

				// Call the delete function (should not crash)
				result.VolumePublication.Delete()
			}

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}

func TestReadVolumePublication(t *testing.T) {
	tests := []struct {
		name                      string
		setupVolumePublication    bool
		volumeID                  string
		nodeID                    string
		expectedVolumePublication *models.VolumePublication
	}{
		{
			name:                   "existing volume publication",
			setupVolumePublication: true,
			volumeID:               "test-volume",
			nodeID:                 "test-node",
			expectedVolumePublication: &models.VolumePublication{
				NodeName:   "test-node",
				VolumeName: "test-volume",
			},
		},
		{
			name:                   "non-existing volume publication",
			setupVolumePublication: false,
			volumeID:               "non-existing-volume",
			nodeID:                 "non-existing-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			if tt.setupVolumePublication {
				volumePublications.data[tt.volumeID+"."+tt.nodeID] = tt.expectedVolumePublication
			}
			volumePublications.unlock()

			// Execute ReadVolumePublication
			subquery := ReadVolumePublication(tt.volumeID, tt.nodeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadVolumePublication setResults should not error")

			// Verify results
			if tt.setupVolumePublication {
				assert.NotNil(t, result.VolumePublication.Read, "VolumePublication should be found")
				assert.Equal(t, tt.expectedVolumePublication, result.VolumePublication.Read, "VolumePublication should match expected")
			} else {
				assert.Nil(t, result.VolumePublication.Read, "VolumePublication should not be found")
			}

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}

func TestListVolumePublications(t *testing.T) {
	tests := []struct {
		name               string
		volumePublications map[string]*models.VolumePublication
		expected           int
	}{
		{
			name:               "empty volume publications",
			volumePublications: map[string]*models.VolumePublication{},
			expected:           0,
		},
		{
			name: "single volume publication",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "node1",
					VolumeName: "vol1",
				},
			},
			expected: 1,
		},
		{
			name: "multiple volume publications",
			volumePublications: map[string]*models.VolumePublication{
				"vol1.node1": {
					NodeName:   "node1",
					VolumeName: "vol1",
				},
				"vol2.node2": {
					NodeName:   "node2",
					VolumeName: "vol2",
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			for k, v := range tt.volumePublications {
				volumePublications.data[k] = v
			}
			volumePublications.unlock()

			// Execute ListVolumePublications
			subquery := ListVolumePublications()
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListVolumePublications setResults should not error")

			// Verify results
			assert.Len(t, result.VolumePublications, tt.expected, "Number of volume publications should match expected")

			// Clean up
			volumePublications.lock()
			volumePublications.data = make(map[string]SmartCopier)
			volumePublications.unlock()
		})
	}
}
