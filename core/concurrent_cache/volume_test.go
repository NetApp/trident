package concurrent_cache

import (
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/storage"
)

func TestUpsertVolumeByInternalName_Metrics(t *testing.T) {
	tests := []struct {
		name            string
		volumeExists    bool
		initialSize     string
		updatedSize     string
		volumeState     storage.VolumeState
		volumeMode      config.VolumeMode
		internalName    string
		newInternalName string
	}{
		{
			name:            "insert new volume",
			volumeExists:    false,
			updatedSize:     "1073741824", // 1GB in bytes
			volumeState:     storage.VolumeStateOnline,
			volumeMode:      config.Filesystem,
			internalName:    "internal-vol-1",
			newInternalName: "internal-vol-1",
		},
		{
			name:            "update existing volume with size increase",
			volumeExists:    true,
			initialSize:     "1073741824", // 1GB in bytes
			updatedSize:     "2147483648", // 2GB in bytes
			volumeState:     storage.VolumeStateOnline,
			volumeMode:      config.Filesystem,
			internalName:    "internal-vol-2",
			newInternalName: "internal-vol-2-updated",
		},
		{
			name:            "update existing volume with different state",
			volumeExists:    true,
			initialSize:     "1073741824", // 1GB in bytes
			updatedSize:     "1073741824", // Same size
			volumeState:     storage.VolumeStateMissingBackend,
			volumeMode:      config.Filesystem,
			internalName:    "internal-vol-3",
			newInternalName: "internal-vol-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

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

			// Setup initial volume if test case requires it
			if tt.volumeExists {
				initialVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:         "test-volume",
						InternalName: tt.internalName,
						Size:         tt.initialSize,
						VolumeMode:   config.Filesystem,
					},
					BackendUUID: "test-backend-uuid",
					State:       storage.VolumeStateOnline,
				}

				// Add volume to volumes map by internal name
				volumes.lock()
				volumes.data[tt.internalName] = initialVolume
				volumes.unlock()

				// Add volume to volumes map by name
				volumes.lock()
				volumes.data["test-volume"] = initialVolume
				volumes.unlock()

				// Add volume to metrics
				addVolumeToMetrics(initialVolume, mockBackend)
			}

			// Get initial metric values for total bytes and volume counts
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(storage.VolumeStateOnline),
				string(config.Filesystem)))

			// Create updated volume
			updatedVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:         "test-volume",
					InternalName: tt.newInternalName,
					Size:         tt.updatedSize,
					VolumeMode:   tt.volumeMode,
				},
				BackendUUID: "test-backend-uuid",
				State:       tt.volumeState,
			}

			// Execute upsert operation
			subquery := UpsertVolumeByInternalName("test-volume", tt.internalName, tt.newInternalName, "test-backend-uuid")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertVolumeByInternalName setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Volume.Upsert, "Upsert function should be created")

			// Call the upsert function with the updated volume
			result.Volume.Upsert(updatedVolume)

			// Parse sizes for comparison
			initialBytes, _ := strconv.ParseFloat(tt.initialSize, 64)
			updatedBytes, _ := strconv.ParseFloat(tt.updatedSize, 64)

			// Get updated metric values
			updatedTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			updatedVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))
			updatedAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))

			if tt.volumeExists {
				if tt.volumeState == storage.VolumeStateOnline && tt.volumeMode == config.Filesystem {
					// If state and mode didn't change, gauge should remain the same
					assert.Equal(t, initialVolumeGauge, updatedVolumeGauge,
						"VolumesGauge should remain the same when state and mode don't change")
				} else {
					// If state or mode changed, old metrics should be 0 and new metrics should be 1
					updatedOldGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
						"test-driver",
						"test-backend-uuid",
						string(storage.VolumeStateOnline),
						string(config.Filesystem)))
					assert.Equal(t, float64(0), updatedOldGauge,
						"Old VolumesGauge should be 0 after update with different state/mode")
					assert.Equal(t, float64(1), updatedVolumeGauge,
						"New VolumesGauge should be 1 after update with different state/mode")
				}

				// Total bytes should reflect the change in size
				expectedTotalBytes := initialTotalBytes - initialBytes + updatedBytes
				assert.InDelta(t, expectedTotalBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should reflect the change in size")

				// Allocated bytes should match the updated volume size
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the updated volume size")
			} else {
				// For new volume
				assert.Equal(t, float64(1), updatedVolumeGauge,
					"VolumesGauge should be 1 for new volume")
				assert.InDelta(t, initialTotalBytes+updatedBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should increase by volume size")
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the volume size")
			}

			// Verify the volume was actually stored in both maps
			volumes.rlock()
			storedVolume, exists := volumes.data[volumes.key.data[tt.newInternalName]]
			volumes.runlock()
			assert.True(t, exists, "Volume should exist in storage after upsert")
			assert.Equal(t, updatedVolume, storedVolume, "Stored volume by internal name should match upserted volume")

			// If internal name changed, old entry should be removed
			if tt.internalName != tt.newInternalName && tt.volumeExists {
				volumes.rlock()
				_, oldEntryExists := volumes.data[volumes.key.data[tt.internalName]]
				volumes.runlock()
				assert.False(t, oldEntryExists, "Old internal name entry should be removed after upsert with new internal name")
			}

			// Clean up
			volumes.lock()
			delete(volumes.data, "test-volume")
			volumes.unlock()

			volumes.lock()
			delete(volumes.key.data, tt.internalName)
			delete(volumes.key.data, tt.newInternalName)
			volumes.unlock()

			backends.lock()
			delete(backends.data, "test-backend-uuid")
			delete(backends.key.data, "test-backend")
			backends.unlock()
		})
	}
}

func TestUpsertVolume_Metrics(t *testing.T) {
	tests := []struct {
		name         string
		volumeExists bool
		initialSize  string
		updatedSize  string
		volumeState  storage.VolumeState
		volumeMode   config.VolumeMode
	}{
		{
			name:         "insert new volume",
			volumeExists: false,
			updatedSize:  "1073741824", // 1GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "update existing volume with size increase",
			volumeExists: true,
			initialSize:  "1073741824", // 1GB in bytes
			updatedSize:  "2147483648", // 2GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "update existing volume with different state",
			volumeExists: true,
			initialSize:  "1073741824", // 1GB in bytes
			updatedSize:  "1073741824", // Same size
			volumeState:  storage.VolumeStateMissingBackend,
			volumeMode:   config.Filesystem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

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

			// Setup initial volume if test case requires it
			if tt.volumeExists {
				initialVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:       "test-volume",
						Size:       tt.initialSize,
						VolumeMode: config.Filesystem,
					},
					BackendUUID: "test-backend-uuid",
					State:       storage.VolumeStateOnline,
				}

				// Add volume to volumes map
				volumes.lock()
				volumes.data["test-volume"] = initialVolume
				volumes.unlock()

				// Add volume to metrics
				addVolumeToMetrics(initialVolume, mockBackend)
			}

			// Get initial metric values for total bytes and volume counts
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(storage.VolumeStateOnline),
				string(config.Filesystem)))

			// Create updated volume
			updatedVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       "test-volume",
					Size:       tt.updatedSize,
					VolumeMode: tt.volumeMode,
				},
				BackendUUID: "test-backend-uuid",
				State:       tt.volumeState,
			}

			// Execute upsert operation
			subquery := UpsertVolume("test-volume", "test-backend-uuid")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertVolume setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Volume.Upsert, "Upsert function should be created")

			// Call the upsert function with the updated volume
			result.Volume.Upsert(updatedVolume)

			// Parse sizes for comparison
			initialBytes, _ := strconv.ParseFloat(tt.initialSize, 64)
			updatedBytes, _ := strconv.ParseFloat(tt.updatedSize, 64)

			// Get updated metric values
			updatedTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			updatedVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))
			updatedAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))

			if tt.volumeExists {
				if tt.volumeState == storage.VolumeStateOnline && tt.volumeMode == config.Filesystem {
					// If state and mode didn't change, gauge should remain the same
					assert.Equal(t, initialVolumeGauge, updatedVolumeGauge,
						"VolumesGauge should remain the same when state and mode don't change")
				} else {
					// If state or mode changed, old metrics should be 0 and new metrics should be 1
					updatedOldGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
						"test-driver",
						"test-backend-uuid",
						string(storage.VolumeStateOnline),
						string(config.Filesystem)))
					assert.Equal(t, float64(0), updatedOldGauge,
						"Old VolumesGauge should be 0 after update with different state/mode")
					assert.Equal(t, float64(1), updatedVolumeGauge,
						"New VolumesGauge should be 1 after update with different state/mode")
				}

				// Total bytes should reflect the change in size
				expectedTotalBytes := initialTotalBytes - initialBytes + updatedBytes
				assert.InDelta(t, expectedTotalBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should reflect the change in size")

				// Allocated bytes should match the updated volume size
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the updated volume size")
			} else {
				// For new volume
				assert.Equal(t, float64(1), updatedVolumeGauge,
					"VolumesGauge should be 1 for new volume")
				assert.InDelta(t, initialTotalBytes+updatedBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should increase by volume size")
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the volume size")
			}

			// Verify the volume was actually stored
			volumes.rlock()
			storedVolume, exists := volumes.data["test-volume"]
			volumes.runlock()
			assert.True(t, exists, "Volume should exist in storage after upsert")
			assert.Equal(t, updatedVolume, storedVolume, "Stored volume should match upserted volume")

			// Clean up
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

func TestUpsertVolumeByBackendName_Metrics(t *testing.T) {
	tests := []struct {
		name         string
		volumeExists bool
		initialSize  string
		updatedSize  string
		volumeState  storage.VolumeState
		volumeMode   config.VolumeMode
	}{
		{
			name:         "insert new volume",
			volumeExists: false,
			updatedSize:  "1073741824", // 1GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "update existing volume with size increase",
			volumeExists: true,
			initialSize:  "1073741824", // 1GB in bytes
			updatedSize:  "2147483648", // 2GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "update existing volume with different state",
			volumeExists: true,
			initialSize:  "1073741824", // 1GB in bytes
			updatedSize:  "1073741824", // Same size
			volumeState:  storage.VolumeStateMissingBackend,
			volumeMode:   config.Filesystem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

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

			// Setup initial volume if test case requires it
			if tt.volumeExists {
				initialVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:       "test-volume",
						Size:       tt.initialSize,
						VolumeMode: config.Filesystem,
					},
					BackendUUID: "test-backend-uuid",
					State:       storage.VolumeStateOnline,
				}

				// Add volume to volumes map
				volumes.lock()
				volumes.data["test-volume"] = initialVolume
				volumes.unlock()

				// Add volume to metrics
				addVolumeToMetrics(initialVolume, mockBackend)
			}

			// Get initial metric values for total bytes and volume counts
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(storage.VolumeStateOnline),
				string(config.Filesystem)))

			// Create updated volume
			updatedVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       "test-volume",
					Size:       tt.updatedSize,
					VolumeMode: tt.volumeMode,
				},
				BackendUUID: "test-backend-uuid",
				State:       tt.volumeState,
			}

			// Execute upsert operation
			subquery := UpsertVolumeByBackendName("test-volume", "test-backend")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertVolumeByBackendName setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Volume.Upsert, "Upsert function should be created")

			// Call the upsert function with the updated volume
			result.Volume.Upsert(updatedVolume)

			// Parse sizes for comparison
			initialBytes, _ := strconv.ParseFloat(tt.initialSize, 64)
			updatedBytes, _ := strconv.ParseFloat(tt.updatedSize, 64)

			// Get updated metric values
			updatedTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			updatedVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))
			updatedAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))

			if tt.volumeExists {
				if tt.volumeState == storage.VolumeStateOnline && tt.volumeMode == config.Filesystem {
					// If state and mode didn't change, gauge should remain the same
					assert.Equal(t, initialVolumeGauge, updatedVolumeGauge,
						"VolumesGauge should remain the same when state and mode don't change")
				} else {
					// If state or mode changed, old metrics should be 0 and new metrics should be 1
					updatedOldGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
						"test-driver",
						"test-backend-uuid",
						string(storage.VolumeStateOnline),
						string(config.Filesystem)))
					assert.Equal(t, float64(0), updatedOldGauge,
						"Old VolumesGauge should be 0 after update with different state/mode")
					assert.Equal(t, float64(1), updatedVolumeGauge,
						"New VolumesGauge should be 1 after update with different state/mode")
				}

				// Total bytes should reflect the change in size
				expectedTotalBytes := initialTotalBytes - initialBytes + updatedBytes
				assert.InDelta(t, expectedTotalBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should reflect the change in size")

				// Allocated bytes should match the updated volume size
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the updated volume size")
			} else {
				// For new volume
				assert.Equal(t, float64(1), updatedVolumeGauge,
					"VolumesGauge should be 1 for new volume")
				assert.InDelta(t, initialTotalBytes+updatedBytes, updatedTotalBytes, 0.1,
					"VolumesTotalBytesGauge should increase by volume size")
				assert.InDelta(t, updatedBytes, updatedAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should match the volume size")
			}

			// Verify the volume was actually stored
			volumes.rlock()
			storedVolume, exists := volumes.data["test-volume"]
			volumes.runlock()
			assert.True(t, exists, "Volume should exist in storage after upsert")
			assert.Equal(t, updatedVolume, storedVolume, "Stored volume should match upserted volume")

			// Clean up
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

func TestDeleteVolume_Metrics(t *testing.T) {
	tests := []struct {
		name         string
		volumeExists bool
		volumeSize   string
		volumeState  storage.VolumeState
		volumeMode   config.VolumeMode
	}{
		{
			name:         "delete existing volume",
			volumeExists: true,
			volumeSize:   "1073741824", // 1GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "delete existing volume with block mode",
			volumeExists: true,
			volumeSize:   "2147483648", // 2GB in bytes
			volumeState:  storage.VolumeStateOnline,
			volumeMode:   config.RawBlock,
		},
		{
			name:         "delete existing volume with different state",
			volumeExists: true,
			volumeSize:   "3221225472", // 3GB in bytes
			volumeState:  storage.VolumeStateMissingBackend,
			volumeMode:   config.Filesystem,
		},
		{
			name:         "delete non-existing volume",
			volumeExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

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

			// Setup initial volume if test case requires it
			var testVolume *storage.Volume
			if tt.volumeExists {
				testVolume = &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:       "test-volume",
						Size:       tt.volumeSize,
						VolumeMode: tt.volumeMode,
					},
					BackendUUID: "test-backend-uuid",
					State:       tt.volumeState,
				}

				// Add volume to volumes map
				volumes.lock()
				volumes.data["test-volume"] = testVolume
				volumes.unlock()

				// Add volume to metrics
				addVolumeToMetrics(testVolume, mockBackend)
			}

			// Get initial metric values
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))
			initialAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
				"test-driver",
				"test-backend-uuid",
				string(tt.volumeState),
				string(tt.volumeMode)))

			// Execute delete operation
			subquery := DeleteVolume("test-volume")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteVolume setResults should not error")

			if tt.volumeExists {
				// Verify the delete function was created and the volume was read
				assert.NotNil(t, result.Volume.Delete, "Delete function should be created")
				assert.Equal(t, testVolume, result.Volume.Read, "Read volume should match the volume that exists")

				// Call the delete function
				result.Volume.Delete()

				// Parse the volume size for comparison
				volumeBytes, _ := strconv.ParseFloat(tt.volumeSize, 64)

				// Verify metrics were updated correctly
				afterDeleteTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
				afterDeleteVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
					"test-driver",
					"test-backend-uuid",
					string(tt.volumeState),
					string(tt.volumeMode)))
				afterDeleteAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
					"test-driver",
					"test-backend-uuid",
					string(tt.volumeState),
					string(tt.volumeMode)))

				assert.Equal(t, initialVolumeGauge-1, afterDeleteVolumeGauge,
					"VolumesGauge should be decremented by 1 when deleting existing volume")
				assert.InDelta(t, initialTotalBytes-volumeBytes, afterDeleteTotalBytes, 0.1,
					"VolumesTotalBytesGauge should be decreased by volume size")
				assert.InDelta(t, initialAllocatedBytes-volumeBytes, afterDeleteAllocatedBytes, 0.1,
					"VolumeAllocatedBytesGauge should be decreased by volume size")

				// Verify the volume was actually removed from storage
				volumes.rlock()
				_, exists := volumes.data["test-volume"]
				volumes.runlock()
				assert.False(t, exists, "Volume should not exist in storage after delete")
			} else {
				// For non-existing volume, delete function should still be created but Read should be nil
				assert.NotNil(t, result.Volume.Delete, "Delete function should be created even for non-existing volume")
				assert.Nil(t, result.Volume.Read, "Read volume should be nil for non-existing volume")

				// Call the delete function
				result.Volume.Delete()

				// Verify metrics were NOT updated (no change since volume didn't exist)
				afterDeleteTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
				afterDeleteVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(
					"test-driver",
					"test-backend-uuid",
					string(tt.volumeState),
					string(tt.volumeMode)))
				afterDeleteAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(
					"test-driver",
					"test-backend-uuid",
					string(tt.volumeState),
					string(tt.volumeMode)))

				assert.Equal(t, initialTotalBytes, afterDeleteTotalBytes,
					"VolumesTotalBytesGauge should remain unchanged when deleting non-existing volume")
				assert.Equal(t, initialVolumeGauge, afterDeleteVolumeGauge,
					"VolumesGauge should remain unchanged when deleting non-existing volume")
				assert.Equal(t, initialAllocatedBytes, afterDeleteAllocatedBytes,
					"VolumeAllocatedBytesGauge should remain unchanged when deleting non-existing volume")
			}

			// Clean up
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
