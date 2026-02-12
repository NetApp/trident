package concurrent_cache

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/storage"
)

func TestUpsertBackend_Metrics(t *testing.T) {
	tests := []struct {
		name           string
		backendExists  bool
		initialBackend storage.Backend
		upsertBackend  storage.Backend
	}{
		{
			name:          "insert new backend",
			backendExists: false,
		},
		{
			name:          "update existing backend",
			backendExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Reset metrics before each test
			metrics.BackendsGauge.Reset()
			metrics.TridentBackendInfo.Reset()

			// Set up initial state if backend exists
			if tt.backendExists {
				tt.initialBackend = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "existing-backend",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       "test-backend-uuid",
				})
				backends.lock()
				backends.data["test-backend-uuid"] = tt.initialBackend
				backends.unlock()
				// Add the existing backend to metrics to simulate realistic state
				addBackendToMetrics(tt.initialBackend)
			}

			// Get initial metric values
			initialBackendGauge := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("test-driver", string(storage.Online)))
			initialTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "existing-backend", "test-backend-uuid"))

			// Create upsert backend
			tt.upsertBackend = getMockBackendWithMap(mockCtrl, map[string]string{
				"name":       "updated-backend",
				"driverName": "test-driver",
				"state":      string(storage.Online),
				"uuid":       "test-backend-uuid",
			})

			// Execute upsert operation
			subquery := UpsertBackend("test-backend-uuid", "test-backend", "updated-backend")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertBackend setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Backend.Upsert, "Upsert function should be created")

			// Call the upsert function
			result.Backend.Upsert(tt.upsertBackend)

			// Verify metrics were updated correctly
			afterUpsertBackendGauge := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("test-driver", string(storage.Online)))
			afterUpsertTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "updated-backend", "test-backend-uuid"))

			if tt.backendExists {
				// For existing backend: delete old (dec) + add new (inc) = no net change
				assert.Equal(t, initialBackendGauge, afterUpsertBackendGauge, "BackendGauge should remain unchanged when updating existing backend")
				// TridentBackendInfo should be set to 1 for the new backend name/labels
				assert.Equal(t, initialTridentBackendInfo, afterUpsertTridentBackendInfo, "TridentBackendInfo should be set to 1 for updated backend")

				// Verify the old TridentBackendInfo metric is removed
				oldTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "existing-backend", "test-backend-uuid"))

				assert.Equal(t, float64(0), oldTridentBackendInfo, "Old TridentBackendInfo should be removed")
			} else {
				// For new backend: only add (inc) = increment by 1
				assert.Equal(t, initialBackendGauge+1, afterUpsertBackendGauge, "BackendGauge should be incremented by 1 when adding new backend")
				assert.Equal(t, initialTridentBackendInfo+1, afterUpsertTridentBackendInfo, "TridentBackendInfo should be set to 1 for new backend")
			}

			// Verify the backend was actually stored
			backends.rlock()
			storedBackend, exists := backends.data["test-backend-uuid"]
			backends.runlock()
			assert.True(t, exists, "Backend should exist in storage after upsert")
			assert.Equal(t, tt.upsertBackend, storedBackend, "Stored backend should match upserted backend")

			// Clean up
			backends.lock()
			delete(backends.data, "test-backend-uuid")
			delete(backends.key.data, "test-backend")
			delete(backends.key.data, "updated-backend")
			backends.unlock()
		})
	}
}

func TestDeleteBackend_Metrics(t *testing.T) {
	tests := []struct {
		name            string
		backendExists   bool
		backendToDelete storage.Backend
	}{
		{
			name:          "delete existing backend",
			backendExists: true,
		},
		{
			name:          "delete non-existing backend",
			backendExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Reset metrics before each test
			metrics.BackendsGauge.Reset()
			metrics.TridentBackendInfo.Reset()

			// Set up initial state if backend exists
			if tt.backendExists {
				tt.backendToDelete = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "existing-backend",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       "test-backend-uuid",
				})
				backends.lock()
				backends.data["test-backend-uuid"] = tt.backendToDelete
				backends.unlock()
				// Add the existing backend to metrics to simulate realistic state
				addBackendToMetrics(tt.backendToDelete)
			}

			// Get initial metric value
			initialBackendGauge := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("test-driver", string(storage.Online)))
			initialTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "existing-backend", "test-backend-uuid"))

			// Execute delete operation
			subquery := DeleteBackend("test-backend-uuid")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteBackend setResults should not error")

			if tt.backendExists {
				// Verify the delete function was created and the backend was read
				assert.NotNil(t, result.Backend.Delete, "Delete function should be created")
				assert.Equal(t, tt.backendToDelete, result.Backend.Read, "Read backend should match the backend that exists")

				// Call the delete function
				result.Backend.Delete()

				// Verify metrics were updated correctly (decremented by 1)
				afterDeleteBackendGauge := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("test-driver", string(storage.Online)))
				afterDeleteTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "existing-backend", "test-backend-uuid"))

				assert.Equal(t, initialBackendGauge-1, afterDeleteBackendGauge, "BackendGauge should be decremented by 1 when deleting existing backend")
				assert.Equal(t, initialTridentBackendInfo-1, afterDeleteTridentBackendInfo, "TridentBackendInfo should be set to 0 when deleting existing backend")

				// Verify the backend was actually removed from storage
				backends.rlock()
				_, exists := backends.data["test-backend-uuid"]
				backends.runlock()
				assert.False(t, exists, "Backend should not exist in storage after delete")
			} else {
				// For non-existing backend, delete function should still be created but Read should be nil
				assert.NotNil(t, result.Backend.Delete, "Delete function should be created even for non-existing backend")
				assert.Nil(t, result.Backend.Read, "Read backend should be nil for non-existing backend")

				// Call the delete function
				result.Backend.Delete()

				// Verify metrics were NOT updated (no change since backend didn't exist)
				afterDeleteBackendGauge := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("test-driver", string(storage.Online)))
				afterDeleteTridentBackendInfo := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues("test-driver", "existing-backend", "test-backend-uuid"))

				assert.Equal(t, initialBackendGauge, afterDeleteBackendGauge, "BackendGauge should remain unchanged when deleting non-existing backend")
				assert.Equal(t, initialTridentBackendInfo, afterDeleteTridentBackendInfo, "TridentBackendInfo should remain unchanged when deleting non-existing backend")
			}
		})
	}
}

func TestListBackends(t *testing.T) {
	tests := []struct {
		name     string
		backends map[string]storage.Backend
		expected int
	}{
		{
			name:     "empty backends",
			backends: map[string]storage.Backend{},
			expected: 0,
		},
		{
			name: "single backend",
			backends: map[string]storage.Backend{
				"backend1": getMockBackendWithMap(gomock.NewController(t), map[string]string{
					"name":       "backend1",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       "backend-uuid-1",
				}),
			},
			expected: 1,
		},
		{
			name: "multiple backends",
			backends: map[string]storage.Backend{
				"backend1": getMockBackendWithMap(gomock.NewController(t), map[string]string{
					"name":       "backend1",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       "backend-uuid-1",
				}),
				"backend2": getMockBackendWithMap(gomock.NewController(t), map[string]string{
					"name":       "backend2",
					"driverName": "test-driver-2",
					"state":      string(storage.Offline),
					"uuid":       "backend-uuid-2",
				}),
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			for k, v := range tt.backends {
				backends.data[k] = v
			}
			backends.unlock()

			// Execute ListBackends
			subquery := ListBackends()
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListBackends setResults should not error")

			// Verify results
			assert.Len(t, result.Backends, tt.expected, "Number of backends should match expected")

			// Clean up
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.unlock()
		})
	}
}

func TestReadBackend(t *testing.T) {
	tests := []struct {
		name            string
		setupBackend    bool
		backendID       string
		expectedBackend storage.Backend
	}{
		{
			name:         "existing backend",
			setupBackend: true,
			backendID:    "test-backend-uuid",
		},
		{
			name:         "non-existing backend",
			setupBackend: false,
			backendID:    "non-existing-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Set up initial state
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			if tt.setupBackend {
				tt.expectedBackend = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "test-backend",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       tt.backendID,
				})
				backends.data[tt.backendID] = tt.expectedBackend
			}
			backends.unlock()

			// Execute ReadBackend
			subquery := ReadBackend(tt.backendID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadBackend setResults should not error")

			// Verify results
			if tt.setupBackend {
				assert.NotNil(t, result.Backend.Read, "Backend should be found")
				assert.Equal(t, tt.expectedBackend, result.Backend.Read, "Backend should match expected")
			} else {
				assert.Nil(t, result.Backend.Read, "Backend should not be found")
			}

			// Clean up
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.unlock()
		})
	}
}

func TestInconsistentReadBackend(t *testing.T) {
	tests := []struct {
		name            string
		setupBackend    bool
		backendID       string
		expectedBackend storage.Backend
	}{
		{
			name:         "existing backend",
			setupBackend: true,
			backendID:    "test-backend-uuid",
		},
		{
			name:         "non-existing backend",
			setupBackend: false,
			backendID:    "non-existing-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Set up initial state
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			if tt.setupBackend {
				tt.expectedBackend = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "test-backend",
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       tt.backendID,
				})
				backends.data[tt.backendID] = tt.expectedBackend
			}
			backends.unlock()

			// Execute InconsistentReadBackend
			subquery := InconsistentReadBackend(tt.backendID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadBackend setResults should not error")

			// Verify results
			if tt.setupBackend {
				assert.NotNil(t, result.Backend.Read, "Backend should be found")
				assert.Equal(t, tt.expectedBackend, result.Backend.Read, "Backend should match expected")
			} else {
				assert.Nil(t, result.Backend.Read, "Backend should not be found")
			}

			// Clean up
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.unlock()
		})
	}
}

func TestReadBackendByName(t *testing.T) {
	tests := []struct {
		name            string
		setupBackend    bool
		backendName     string
		backendID       string
		expectedBackend storage.Backend
	}{
		{
			name:         "existing backend by name",
			setupBackend: true,
			backendName:  "test-backend-name",
			backendID:    "test-backend-uuid",
		},
		{
			name:         "non-existing backend by name",
			setupBackend: false,
			backendName:  "non-existing-name",
			backendID:    "non-existing-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Set up initial state
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.key.data = make(map[string]string)
			if tt.setupBackend {
				tt.expectedBackend = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       tt.backendName,
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       tt.backendID,
				})
				backends.data[tt.backendID] = tt.expectedBackend
				backends.key.data[tt.backendName] = tt.backendID
			}
			backends.unlock()

			// Execute ReadBackendByName
			subquery := ReadBackendByName(tt.backendName)
			// Simulate fillInIDs processing
			subquery.id = backends.key.data[tt.backendName]
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadBackendByName setResults should not error")

			// Verify results
			if tt.setupBackend {
				assert.NotNil(t, result.Backend.Read, "Backend should be found")
				assert.Equal(t, tt.expectedBackend, result.Backend.Read, "Backend should match expected")
			} else {
				assert.Nil(t, result.Backend.Read, "Backend should not be found")
			}

			// Clean up
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.key.data = make(map[string]string)
			backends.unlock()
		})
	}
}

func TestInconsistentReadBackendByName(t *testing.T) {
	tests := []struct {
		name            string
		setupBackend    bool
		backendName     string
		backendID       string
		expectedBackend storage.Backend
	}{
		{
			name:         "existing backend by name",
			setupBackend: true,
			backendName:  "test-backend-name",
			backendID:    "test-backend-uuid",
		},
		{
			name:         "non-existing backend by name",
			setupBackend: false,
			backendName:  "non-existing-name",
			backendID:    "non-existing-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Set up initial state
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.key.data = make(map[string]string)
			if tt.setupBackend {
				tt.expectedBackend = getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       tt.backendName,
					"driverName": "test-driver",
					"state":      string(storage.Online),
					"uuid":       tt.backendID,
				})
				backends.data[tt.backendID] = tt.expectedBackend
				backends.key.data[tt.backendName] = tt.backendID
			}
			backends.unlock()

			// Execute InconsistentReadBackendByName
			subquery := InconsistentReadBackendByName(tt.backendName)
			// Simulate fillInIDs processing
			subquery.id = backends.key.data[tt.backendName]
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadBackendByName setResults should not error")

			// Verify results
			if tt.setupBackend {
				assert.NotNil(t, result.Backend.Read, "Backend should be found")
				assert.Equal(t, tt.expectedBackend, result.Backend.Read, "Backend should match expected")
			} else {
				assert.Nil(t, result.Backend.Read, "Backend should not be found")
			}

			// Clean up
			backends.lock()
			backends.data = make(map[string]SmartCopier)
			backends.key.data = make(map[string]string)
			backends.unlock()
		})
	}
}
