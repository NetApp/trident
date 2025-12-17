package concurrent_cache

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/storage"
)

func TestUpsertSnapshot_Metrics(t *testing.T) {
	tests := []struct {
		name           string
		snapshotExists bool
		initialSize    int64
		updatedSize    int64
		snapshotState  storage.SnapshotState
	}{
		{
			name:           "insert new snapshot",
			snapshotExists: false,
			updatedSize:    1073741824, // 1GB in bytes
			snapshotState:  storage.SnapshotStateOnline,
		},
		{
			name:           "update existing snapshot with size increase",
			snapshotExists: true,
			initialSize:    1073741824, // 1GB in bytes
			updatedSize:    2147483648, // 2GB in bytes
			snapshotState:  storage.SnapshotStateOnline,
		},
		{
			name:           "update existing snapshot with different state",
			snapshotExists: true,
			initialSize:    1073741824, // 1GB in bytes
			updatedSize:    1073741824, // Same size
			snapshotState:  storage.SnapshotStateMissingBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SnapshotGauge.Reset()
			metrics.SnapshotAllocatedBytesGauge.Reset()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create mock backend and volume
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"name":       "test-backend",
				"driverName": "test-driver",
				"uuid":       "test-backend-uuid",
			})

			// Add backend to backends map
			backends.lock()
			backends.data["test-backend-uuid"] = mockBackend
			backends.key.data["test-backend"] = "test-backend-uuid"
			backends.unlock()

			// Create volume
			testVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       "test-volume",
					Size:       "1073741824",
					VolumeMode: config.Filesystem,
				},
				BackendUUID: "test-backend-uuid",
				State:       storage.VolumeStateOnline,
			}

			// Add volume to volumes map
			volumes.lock()
			volumes.data["test-volume"] = testVolume
			volumes.unlock()

			// Setup initial snapshot if test case requires it
			if tt.snapshotExists {
				initialSnapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{
						Name:       "test-snapshot",
						VolumeName: "test-volume",
					},
					Created:   time.Now().Format(time.RFC3339),
					SizeBytes: tt.initialSize,
					State:     storage.SnapshotStateOnline,
				}

				// Add snapshot to snapshots map
				snapshots.lock()
				snapshots.data["test-volume/test-snapshot"] = initialSnapshot
				snapshots.unlock()

				// Add snapshot to metrics
				addSnapshotToMetrics(initialSnapshot, testVolume, mockBackend)
			}

			// Get initial metric values
			initialSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues("test-driver", "test-backend-uuid"))
			initialAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues("test-driver", "test-backend-uuid"))

			// Create updated snapshot
			updatedSnapshot := &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       "test-snapshot",
					VolumeName: "test-volume",
				},
				Created:   time.Now().Format(time.RFC3339),
				SizeBytes: tt.updatedSize,
				State:     tt.snapshotState,
			}

			// Execute upsert operation
			subquery := UpsertSnapshot("test-volume", "test-volume/test-snapshot")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertSnapshot setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Snapshot.Upsert, "Upsert function should be created")

			// Call the upsert function with the updated snapshot
			result.Snapshot.Upsert(updatedSnapshot)

			// Get updated metric values
			updatedSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues("test-driver", "test-backend-uuid"))
			updatedAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues("test-driver", "test-backend-uuid"))

			if tt.snapshotExists {
				// Gauge should remain the same
				assert.Equal(t, initialSnapshotGauge, updatedSnapshotGauge,
					"SnapshotGauge should remain the same when state doesn't change")
				assert.InDelta(t, initialAllocatedBytes-float64(tt.initialSize)+float64(tt.updatedSize), updatedAllocatedBytes, 0.1,
					"SnapshotAllocatedBytesGauge should match the updated snapshot size")
			} else {
				// For new snapshot
				assert.Equal(t, initialSnapshotGauge+1, updatedSnapshotGauge,
					"SnapshotGauge should be 1 for new snapshot")
				assert.InDelta(t, initialAllocatedBytes+float64(tt.updatedSize), updatedAllocatedBytes, 0.1,
					"SnapshotAllocatedBytesGauge should match the snapshot size")
			}

			// Verify the snapshot was actually stored
			snapshots.rlock()
			storedSnapshot, exists := snapshots.data["test-volume/test-snapshot"]
			snapshots.runlock()
			assert.True(t, exists, "Snapshot should exist in storage after upsert")
			assert.Equal(t, updatedSnapshot, storedSnapshot, "Stored snapshot should match upserted snapshot")

			// Clean up
			snapshots.lock()
			delete(snapshots.data, "test-volume/test-snapshot")
			snapshots.unlock()

			volumes.lock()
			delete(volumes.data, "test-volume")
			volumes.unlock()

			backends.lock()
			delete(backends.data, "test-backend-uuid")
			delete(backends.key.data, "test-backend")
			backends.unlock()
		})
	}
}

func TestDeleteSnapshot_Metrics(t *testing.T) {
	tests := []struct {
		name           string
		snapshotExists bool
		snapshotSize   int64
		snapshotState  storage.SnapshotState
	}{
		{
			name:           "delete existing snapshot",
			snapshotExists: true,
			snapshotSize:   1073741824, // 1GB in bytes
			snapshotState:  storage.SnapshotStateOnline,
		},
		{
			name:           "delete existing snapshot with different state",
			snapshotExists: true,
			snapshotSize:   3221225472, // 3GB in bytes
			snapshotState:  storage.SnapshotStateMissingBackend,
		},
		{
			name:           "delete non-existing snapshot",
			snapshotExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SnapshotGauge.Reset()
			metrics.SnapshotAllocatedBytesGauge.Reset()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create mock backend
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"name":       "test-backend",
				"driverName": "test-driver",
				"uuid":       "test-backend-uuid",
			})

			// Add backend to backends map
			backends.lock()
			backends.data["test-backend-uuid"] = mockBackend
			backends.key.data["test-backend"] = "test-backend-uuid"
			backends.unlock()

			// Create volume
			testVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       "test-volume",
					Size:       "1073741824",
					VolumeMode: config.Filesystem,
				},
				BackendUUID: "test-backend-uuid",
				State:       storage.VolumeStateOnline,
			}

			// Add volume to volumes map
			volumes.lock()
			volumes.data["test-volume"] = testVolume
			volumes.unlock()

			// Setup initial snapshot if test case requires it
			var testSnapshot *storage.Snapshot
			if tt.snapshotExists {
				testSnapshot = &storage.Snapshot{
					Config: &storage.SnapshotConfig{
						Name:       "test-snapshot",
						VolumeName: "test-volume",
					},
					Created:   time.Now().Format(time.RFC3339),
					SizeBytes: tt.snapshotSize,
					State:     tt.snapshotState,
				}

				// Add snapshot to snapshots map
				snapshots.lock()
				snapshots.data["test-volume/test-snapshot"] = testSnapshot
				snapshots.unlock()

				// Add snapshot to metrics
				addSnapshotToMetrics(testSnapshot, testVolume, mockBackend)
			}

			// Get initial metric values
			initialSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues("test-driver", "test-backend-uuid"))
			initialAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues("test-driver", "test-backend-uuid"))

			// Execute delete operation
			subquery := DeleteSnapshot("test-volume/test-snapshot")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteSnapshot setResults should not error")

			if tt.snapshotExists {
				// Verify the delete function was created and the snapshot was read
				assert.NotNil(t, result.Snapshot.Delete, "Delete function should be created")
				assert.Equal(t, testSnapshot, result.Snapshot.Read, "Read snapshot should match the snapshot that exists")

				// Call the delete function
				result.Snapshot.Delete()

				// Verify metrics were updated correctly
				afterDeleteSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues("test-driver", "test-backend-uuid"))
				afterDeleteAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues("test-driver", "test-backend-uuid"))

				assert.Equal(t, initialSnapshotGauge-1, afterDeleteSnapshotGauge,
					"SnapshotGauge should be decremented by 1 when deleting existing snapshot")
				assert.Equal(t, initialAllocatedBytes-float64(tt.snapshotSize), afterDeleteAllocatedBytes,
					"SnapshotAllocatedBytesGauge should be decreased by snapshot size")

				// Verify the snapshot was actually removed from storage
				snapshots.rlock()
				_, exists := snapshots.data["test-volume/test-snapshot"]
				snapshots.runlock()
				assert.False(t, exists, "Snapshot should not exist in storage after delete")
			} else {
				// For non-existing snapshot, delete function should still be created but Read should be nil
				assert.NotNil(t, result.Snapshot.Delete, "Delete function should be created even for non-existing snapshot")
				assert.Nil(t, result.Snapshot.Read, "Read snapshot should be nil for non-existing snapshot")

				// Call the delete function
				result.Snapshot.Delete()

				// Verify metrics were NOT updated (no change since snapshot didn't exist)
				afterDeleteSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues("test-driver", "test-backend-uuid"))
				afterDeleteAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues("test-driver", "test-backend-uuid"))

				assert.Equal(t, initialSnapshotGauge, afterDeleteSnapshotGauge,
					"SnapshotGauge should remain unchanged when deleting non-existing snapshot")
				assert.Equal(t, initialAllocatedBytes, afterDeleteAllocatedBytes,
					"SnapshotAllocatedBytesGauge should remain unchanged when deleting non-existing snapshot")
			}

			// Clean up
			snapshots.lock()
			delete(snapshots.data, "test-volume/test-snapshot")
			snapshots.unlock()

			volumes.lock()
			delete(volumes.data, "test-volume")
			volumes.unlock()

			backends.lock()
			delete(backends.data, "test-backend-uuid")
			delete(backends.key.data, "test-backend")
			backends.unlock()
		})
	}
}

func TestListSnapshots(t *testing.T) {
	tests := []struct {
		name      string
		snapshots map[string]*storage.Snapshot
		expected  int
	}{
		{
			name:      "empty snapshots",
			snapshots: map[string]*storage.Snapshot{},
			expected:  0,
		},
		{
			name: "single snapshot",
			snapshots: map[string]*storage.Snapshot{
				"snapshot1": {
					Config: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "volume1",
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple snapshots",
			snapshots: map[string]*storage.Snapshot{
				"snapshot1": {
					Config: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "volume1",
					},
				},
				"snapshot2": {
					Config: &storage.SnapshotConfig{
						Name:       "snapshot2",
						VolumeName: "volume2",
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			for k, v := range tt.snapshots {
				snapshots.data[k] = v
			}
			snapshots.unlock()

			// Execute ListSnapshots
			subquery := ListSnapshots()
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSnapshots setResults should not error")

			// Verify results
			assert.Len(t, result.Snapshots, tt.expected, "Number of snapshots should match expected")

			// Clean up
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			snapshots.unlock()
		})
	}
}

func TestListSnapshotsByName(t *testing.T) {
	tests := []struct {
		name         string
		snapshots    map[string]*storage.Snapshot
		snapshotName string
		expected     int
	}{
		{
			name:         "no matching snapshots",
			snapshots:    map[string]*storage.Snapshot{},
			snapshotName: "nonexistent",
			expected:     0,
		},
		{
			name: "single matching snapshot",
			snapshots: map[string]*storage.Snapshot{
				"snapshot1": {
					Config: &storage.SnapshotConfig{
						Name:       "target-snapshot",
						VolumeName: "volume1",
					},
				},
				"snapshot2": {
					Config: &storage.SnapshotConfig{
						Name:       "other-snapshot",
						VolumeName: "volume2",
					},
				},
			},
			snapshotName: "target-snapshot",
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			for k, v := range tt.snapshots {
				snapshots.data[k] = v
			}
			snapshots.unlock()

			// Execute ListSnapshotsByName
			subquery := ListSnapshotsByName(tt.snapshotName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSnapshotsByName setResults should not error")

			// Verify results
			assert.Len(t, result.Snapshots, tt.expected, "Number of snapshots should match expected")
			for _, snapshot := range result.Snapshots {
				assert.Equal(t, tt.snapshotName, snapshot.Config.Name, "Snapshot name should match filter")
			}

			// Clean up
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			snapshots.unlock()
		})
	}
}

func TestListSnapshotsForVolume(t *testing.T) {
	tests := []struct {
		name       string
		snapshots  map[string]*storage.Snapshot
		volumeName string
		expected   int
	}{
		{
			name:       "no matching snapshots",
			snapshots:  map[string]*storage.Snapshot{},
			volumeName: "nonexistent-volume",
			expected:   0,
		},
		{
			name: "single matching snapshot",
			snapshots: map[string]*storage.Snapshot{
				"snapshot1": {
					Config: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "target-volume",
					},
				},
				"snapshot2": {
					Config: &storage.SnapshotConfig{
						Name:       "snapshot2",
						VolumeName: "other-volume",
					},
				},
			},
			volumeName: "target-volume",
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			for k, v := range tt.snapshots {
				snapshots.data[k] = v
			}
			snapshots.unlock()

			// Execute ListSnapshotsForVolume
			subquery := ListSnapshotsForVolume(tt.volumeName)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListSnapshotsForVolume setResults should not error")

			// Verify results
			assert.Len(t, result.Snapshots, tt.expected, "Number of snapshots should match expected")
			for _, snapshot := range result.Snapshots {
				assert.Equal(t, tt.volumeName, snapshot.Config.VolumeName, "Snapshot volume name should match filter")
			}

			// Clean up
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			snapshots.unlock()
		})
	}
}

func TestReadSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		setupSnapshot    bool
		snapshotID       string
		expectedSnapshot *storage.Snapshot
	}{
		{
			name:          "existing snapshot",
			setupSnapshot: true,
			snapshotID:    "test-snapshot-id",
			expectedSnapshot: &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       "test-snapshot",
					VolumeName: "test-volume",
				},
			},
		},
		{
			name:          "non-existing snapshot",
			setupSnapshot: false,
			snapshotID:    "non-existing-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			if tt.setupSnapshot {
				snapshots.data[tt.snapshotID] = tt.expectedSnapshot
			}
			snapshots.unlock()

			// Execute ReadSnapshot
			subquery := ReadSnapshot(tt.snapshotID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadSnapshot setResults should not error")

			// Verify results
			if tt.setupSnapshot {
				assert.NotNil(t, result.Snapshot.Read, "Snapshot should be found")
				assert.Equal(t, tt.expectedSnapshot, result.Snapshot.Read, "Snapshot should match expected")
			} else {
				assert.Nil(t, result.Snapshot.Read, "Snapshot should not be found")
			}

			// Clean up
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			snapshots.unlock()
		})
	}
}

func TestInconsistentReadSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		setupSnapshot    bool
		snapshotID       string
		expectedSnapshot *storage.Snapshot
	}{
		{
			name:          "existing snapshot",
			setupSnapshot: true,
			snapshotID:    "test-snapshot-id",
			expectedSnapshot: &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       "test-snapshot",
					VolumeName: "test-volume",
				},
			},
		},
		{
			name:          "non-existing snapshot",
			setupSnapshot: false,
			snapshotID:    "non-existing-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			if tt.setupSnapshot {
				snapshots.data[tt.snapshotID] = tt.expectedSnapshot
			}
			snapshots.unlock()

			// Execute InconsistentReadSnapshot
			subquery := InconsistentReadSnapshot(tt.snapshotID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadSnapshot setResults should not error")

			// Verify results
			if tt.setupSnapshot {
				assert.NotNil(t, result.Snapshot.Read, "Snapshot should be found")
				assert.Equal(t, tt.expectedSnapshot, result.Snapshot.Read, "Snapshot should match expected")
			} else {
				assert.Nil(t, result.Snapshot.Read, "Snapshot should not be found")
			}

			// Clean up
			snapshots.lock()
			snapshots.data = make(map[string]SmartCopier)
			snapshots.unlock()
		})
	}
}
