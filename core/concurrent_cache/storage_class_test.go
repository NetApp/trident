package concurrent_cache

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/core/metrics"
	storageclass "github.com/netapp/trident/storage_class"
)

func TestUpsertStorageClass_Metrics(t *testing.T) {
	tests := []struct {
		name               string
		storageClassExists bool
		initialSC          *storageclass.StorageClass
		upsertSC           *storageclass.StorageClass
	}{
		{
			name:               "insert new storage class",
			storageClassExists: false,
			upsertSC: storageclass.New(&storageclass.Config{
				Name: "new-sc",
			}),
		},
		{
			name:               "update existing storage class",
			storageClassExists: true,
			initialSC: storageclass.New(&storageclass.Config{
				Name: "existing-sc",
			}),
			upsertSC: storageclass.New(&storageclass.Config{
				Name: "existing-sc-updated",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SCGauge.Set(0)

			// Set up initial state if storage class exists
			if tt.storageClassExists {
				storageClasses.lock()
				storageClasses.data["test-sc"] = tt.initialSC
				storageClasses.unlock()
				// Add the existing storage class to metrics to simulate realistic state
				addStorageClassToMetrics(tt.initialSC)
			}

			// Get initial metric value
			initialSCGauge := testutil.ToFloat64(metrics.SCGauge)

			// Execute upsert operation
			subquery := UpsertStorageClass("test-sc")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertStorageClass setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.StorageClass.Upsert, "Upsert function should be created")

			// Call the upsert function
			result.StorageClass.Upsert(tt.upsertSC)

			// Verify metrics were updated correctly
			afterUpsertSCGauge := testutil.ToFloat64(metrics.SCGauge)

			if tt.storageClassExists {
				// For existing storage class: delete old (dec) + add new (inc) = no net change
				assert.Equal(t, initialSCGauge, afterUpsertSCGauge, "SCGauge should remain unchanged when updating existing storage class")
			} else {
				// For new storage class: only add (inc) = increment by 1
				assert.Equal(t, initialSCGauge+1, afterUpsertSCGauge, "SCGauge should be incremented by 1 when adding new storage class")
			}

			// Verify the storage class was actually stored
			storageClasses.rlock()
			storedSC, exists := storageClasses.data["test-sc"]
			storageClasses.runlock()
			assert.True(t, exists, "Storage class should exist in storage after upsert")
			assert.Equal(t, tt.upsertSC, storedSC, "Stored storage class should match upserted storage class")

			// Clean up
			storageClasses.lock()
			delete(storageClasses.data, "test-sc")
			storageClasses.unlock()
		})
	}
}

func TestDeleteStorageClass_Metrics(t *testing.T) {
	tests := []struct {
		name               string
		storageClassExists bool
		scToDelete         *storageclass.StorageClass
	}{
		{
			name:               "delete existing storage class",
			storageClassExists: true,
			scToDelete: storageclass.New(&storageclass.Config{
				Name: "existing-sc",
			}),
		},
		{
			name:               "delete non-existing storage class",
			storageClassExists: false,
			scToDelete:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SCGauge.Set(0)

			// Set up initial state if storage class exists
			if tt.storageClassExists {
				storageClasses.lock()
				storageClasses.data["test-sc"] = tt.scToDelete
				storageClasses.unlock()
				// Add the existing storage class to metrics to simulate realistic state
				addStorageClassToMetrics(tt.scToDelete)
			}

			// Get initial metric value
			initialSCGauge := testutil.ToFloat64(metrics.SCGauge)

			// Execute delete operation
			subquery := DeleteStorageClass("test-sc")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteStorageClass setResults should not error")

			if tt.storageClassExists {
				// Verify the delete function was created and the storage class was read
				assert.NotNil(t, result.StorageClass.Delete, "Delete function should be created")
				assert.Equal(t, tt.scToDelete, result.StorageClass.Read, "Read storage class should match the storage class that exists")

				// Call the delete function
				result.StorageClass.Delete()

				// Verify metrics were updated correctly (decremented by 1)
				afterDeleteSCGauge := testutil.ToFloat64(metrics.SCGauge)
				assert.Equal(t, initialSCGauge-1, afterDeleteSCGauge, "SCGauge should be decremented by 1 when deleting existing storage class")

				// Verify the storage class was actually removed from storage
				storageClasses.rlock()
				_, exists := storageClasses.data["test-sc"]
				storageClasses.runlock()
				assert.False(t, exists, "Storage class should not exist in storage after delete")
			} else {
				// For non-existing storage class, delete function should still be created but Read should be nil
				assert.NotNil(t, result.StorageClass.Delete, "Delete function should be created even for non-existing storage class")
				assert.Nil(t, result.StorageClass.Read, "Read storage class should be nil for non-existing storage class")

				// Call the delete function
				result.StorageClass.Delete()

				// Verify metrics were NOT updated (no change since storage class didn't exist)
				afterDeleteSCGauge := testutil.ToFloat64(metrics.SCGauge)
				assert.Equal(t, initialSCGauge, afterDeleteSCGauge, "SCGauge should remain unchanged when deleting non-existing storage class")
			}
		})
	}
}
