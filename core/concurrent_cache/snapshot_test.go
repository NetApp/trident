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
