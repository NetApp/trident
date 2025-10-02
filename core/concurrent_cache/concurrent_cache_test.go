package concurrent_cache

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

func TestDedupe(t *testing.T) {
	t.Run("dedupe error", func(t *testing.T) {
		query := Query(
			ListVolumePublications(),
			UpsertNode(""),
			ReadVolumePublication("", ""),
			UpsertVolume("", ""),
			DeleteBackend(""),
			ReadBackend(""),
		)
		_, err := dedupe(query)
		assert.Error(t, err)
	})
	t.Run("dedupe error with list", func(t *testing.T) {
		query := Query(
			ListVolumePublications(),
			UpsertNode(""),
			ListVolumePublications(),
			UpsertVolume("", ""),
			DeleteBackend(""),
		)
		_, err := dedupe(query)
		assert.Error(t, err)
	})
	t.Run("dedupe success with list", func(t *testing.T) {
		query := Query(
			ListVolumePublications(),
			UpsertNode(""),
			UpsertVolumePublication("", ""),
			UpsertVolume("", ""),
			DeleteBackend(""),
			ListVolumes(),
		)
		_, err := dedupe(query)
		assert.NoError(t, err)
	})
	t.Run("dedupe success with list one resource", func(t *testing.T) {
		query := Query(
			ListVolumePublications(),
			ReadVolumePublication("", ""),
		)
		_, err := dedupe(query)
		assert.NoError(t, err)
	})
}

func TestBuildTrees(t *testing.T) {
	tests := []struct {
		name     string
		query    []Subquery
		queryLen int
		rootLen  int
	}{
		{
			"build tree success",
			Query(
				ListVolumePublications(),
				UpsertNode(""),
				UpsertVolumePublication("", ""),
				ReadVolume(""),
				ReadBackend(""),
			),
			5,
			2,
		},
		{
			"build tree success with lists",
			Query(
				ListVolumePublications(),
				UpsertNode(""),
				UpsertVolumePublication("", ""),
				ReadBackend(""),
				ListVolumes(),
			),
			6,
			3,
		},
		{
			"build tree with one subquery",
			Query(
				UpsertVolumePublication("volume1", "node1"),
			),
			4,
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := dedupe(tt.query)
			assert.NoError(t, err)
			queries, roots := buildTrees(m, tt.query)
			assert.Len(t, queries, tt.queryLen)
			assert.Len(t, roots, tt.rootLen)
		})
	}
}

func TestBuildTree(t *testing.T) {
	query := Query(
		ListVolumePublications(),
		UpsertNode(""),
		UpsertVolumePublication("", ""),
		ReadBackend(""),
		ListVolumes(),
	)
	m, err := dedupe(query)
	assert.NoError(t, err)
	query = buildTree(m, query, 4)
	assert.Len(t, query, 6)
}

func TestFillInIDs(t *testing.T) {
	tests := []struct {
		name  string
		query []Subquery
	}{
		{
			"fill in ids success",
			Query(
				ListVolumePublications(),
				UpsertNode(""),
				UpsertVolumePublication("volume1", "node1"),
				ReadVolume(""),
				ReadBackend(""),
			),
		},
		{
			"fill in ids success with lists",
			Query(
				ListVolumePublications(),
				UpsertNode(""),
				UpsertVolumePublication("volume1", "node1"),
				ReadBackend(""),
				ListVolumes(),
			),
		},
		{
			"fill in ids with one subquery",
			Query(
				UpsertVolumePublication("volume1", "node1"),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initCaches()
			defer cleanCaches()
			m, err := dedupe(tt.query)
			assert.NoError(t, err)
			query, roots := buildTrees(m, tt.query)
			for _, r := range roots {
				err := fillInIDs(r, query)
				assert.NoError(t, err)
			}
			for _, q := range query {
				if q.op != list {
					assert.NotEmpty(t, q.id)
				} else {
					assert.Empty(t, q.id)
				}
			}
		})
	}
}

func TestMergeQueries(t *testing.T) {
	tests := []struct {
		name    string
		queries [][]Subquery
	}{
		{
			"merge queries success",
			[][]Subquery{
				{
					ListVolumePublications(),
					UpsertNode(""),
					UpsertVolumePublication("volume1", "node1"),
					ReadVolume(""),
					ReadBackend(""),
				},
				{
					ListVolumePublications(),
					UpsertNode("node1"),
					ReadBackend("backend1"),
					ListVolumes(),
				},
				{
					UpsertVolumePublication("volume1", "node2"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initCaches()
			nodes.data["node2"] = &models.Node{Name: "node2"}
			defer cleanCaches()

			queries := make([][]Subquery, 0, len(tt.queries))
			for _, q := range tt.queries {
				m, err := dedupe(q)
				assert.NoError(t, err)
				q, roots := buildTrees(m, q)
				for _, r := range roots {
					err := fillInIDs(r, q)
					assert.NoError(t, err)
				}
				queries = append(queries, q)
			}

			merged := mergeQueries(queries)
			assert.Len(t, merged, 13)
		})
	}
}

func TestLock(t *testing.T) {
	// start two goroutines that try to update the same resources using multiple queries
	// passes if there is no deadlock
	initCaches()
	defer cleanCaches()
	makeQueries := func() [][]Subquery {
		return [][]Subquery{
			{
				ListVolumePublications(),
				UpsertNode(""),
				UpsertVolumePublication("volume1", "node1"),
				ReadVolume(""),
				ReadBackend(""),
			},
			{
				ListVolumePublications(),
				UpsertNode("node1"),
				ReadBackend("backend1"),
				ListVolumes(),
			},
			{
				UpsertVolumePublication("volume1", "node2"),
			},
		}
	}

	wg := sync.WaitGroup{}
	n := 2
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			results, unlocker, err := Lock(context.Background(), makeQueries()...)
			defer unlocker()
			assert.NoError(t, err)
			assert.NotNil(t, results[2].VolumePublication.Upsert)
			time.Sleep(1 * time.Second)
			var vp *models.VolumePublication
			if results[2].VolumePublication.Read != nil {
				vp = results[2].VolumePublication.Read
				vp.AccessMode++
			} else {
				vp = &models.VolumePublication{
					NodeName:   "node2",
					VolumeName: "volume1",
					AccessMode: 1,
				}
			}
			results[2].VolumePublication.Upsert(vp)
		}()
	}
	wg.Wait()

	// check that the volume publication was created and access mode equals n
	vp, ok := volumePublications.data["volume1.node2"].(*models.VolumePublication)
	assert.True(t, ok)
	assert.Equal(t, n, int(vp.AccessMode))
}

func initCaches() {
	nodes.data = make(map[string]SmartCopier)
	backends.data = make(map[string]SmartCopier)
	volumes.data = make(map[string]SmartCopier)
	volumePublications.data = make(map[string]SmartCopier)
	snapshots.data = make(map[string]SmartCopier)

	nodes.data["node1"] = &models.Node{}
	backends.data["backend1"] = &storage.StorageBackend{}
	volumes.data["volume1"] = &storage.Volume{
		BackendUUID: "backend1",
		Config: &storage.VolumeConfig{
			Name:         "volume1",
			InternalName: "internal-volume1",
		},
	}
	volumePublications.data["volume1.node1"] = &models.VolumePublication{
		NodeName:   "node1",
		VolumeName: "volume1",
	}
	snapshots.data["snapshot1"] = &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			VolumeName: "volume1",
		},
	}
}

func cleanCaches() {
	nodes.data = make(map[string]SmartCopier)
	backends.data = make(map[string]SmartCopier)
	volumes.data = make(map[string]SmartCopier)
	volumePublications.data = make(map[string]SmartCopier)
	snapshots.data = make(map[string]SmartCopier)
}

func createQueryGenerators(numIds int) [][]queryGenerator {
	generators := make([][]queryGenerator, 0, numIds)

	for i := 0; i < numIds; i++ {
		backendName := fmt.Sprintf("backend%d", i+1)
		volumeName := fmt.Sprintf("volume%d", i+1)
		snapshotName := fmt.Sprintf("snapshot%d", i+1)
		nodeName := fmt.Sprintf("node%d", i+1)
		storageClassName := fmt.Sprintf("sc%d", i+1)
		subordinateVolumeName := fmt.Sprintf("subvol%d", i+1)
		generators = append(generators, []queryGenerator{
			// Backend queries
			{name: "ListBackends", fn: ListBackends},
			{name: "InconsistentReadBackend", fn: func() Subquery { return InconsistentReadBackend(backendName) }},
			{name: "ReadBackend", fn: func() Subquery { return ReadBackend(backendName) }},
			{name: "UpsertBackend", fn: func() Subquery { return UpsertBackend(backendName, backendName, "") }},
			{name: "DeleteBackend", fn: func() Subquery { return DeleteBackend(backendName) }},

			// Volume queries
			{name: "ListVolumes", fn: ListVolumes},
			{name: "InconsistentReadVolume", fn: func() Subquery { return InconsistentReadVolume(volumeName) }},
			{name: "ReadVolume", fn: func() Subquery { return ReadVolume(volumeName) }},
			{name: "UpsertVolume", fn: func() Subquery { return UpsertVolume(volumeName, backendName) }},
			{name: "DeleteVolume", fn: func() Subquery { return DeleteVolume(volumeName) }},

			// Snapshot queries
			{name: "ListSnapshots", fn: ListSnapshots},
			{name: "InconsistentReadSnapshot", fn: func() Subquery { return InconsistentReadSnapshot(snapshotName) }},
			{name: "ReadSnapshot", fn: func() Subquery { return ReadSnapshot(snapshotName) }},
			{name: "UpsertSnapshot", fn: func() Subquery { return UpsertSnapshot(volumeName, snapshotName) }},
			{name: "DeleteSnapshot", fn: func() Subquery { return DeleteSnapshot(snapshotName) }},

			// Node queries
			{name: "ListNodes", fn: ListNodes},
			{name: "InconsistentReadNode", fn: func() Subquery { return InconsistentReadNode(nodeName) }},
			{name: "ReadNode", fn: func() Subquery { return ReadNode(nodeName) }},
			{name: "UpsertNode", fn: func() Subquery { return UpsertNode(nodeName) }},
			{name: "DeleteNode", fn: func() Subquery { return DeleteNode(nodeName) }},

			// StorageClass queries
			{name: "ListStorageClasses", fn: ListStorageClasses},
			{name: "InconsistentReadStorageClass", fn: func() Subquery { return InconsistentReadStorageClass(storageClassName) }},
			{name: "ReadStorageClass", fn: func() Subquery { return ReadStorageClass(storageClassName) }},
			{name: "UpsertStorageClass", fn: func() Subquery { return UpsertStorageClass(storageClassName) }},
			{name: "DeleteStorageClass", fn: func() Subquery { return DeleteStorageClass(storageClassName) }},

			// VolumePublication queries
			{name: "ListVolumePublications", fn: ListVolumePublications},
			{name: "InconsistentReadVolumePublication", fn: func() Subquery { return InconsistentReadVolumePublication(volumeName, nodeName) }},
			{name: "ReadVolumePublication", fn: func() Subquery { return ReadVolumePublication(volumeName, nodeName) }},
			{name: "UpsertVolumePublication", fn: func() Subquery { return UpsertVolumePublication(volumeName, nodeName) }},
			{name: "DeleteVolumePublication", fn: func() Subquery { return DeleteVolumePublication(volumeName, nodeName) }},

			// SubordinateVolume queries
			{name: "ListSubordinateVolumes", fn: ListSubordinateVolumes},
			{name: "ListSubordinateVolumesForVolume", fn: func() Subquery {
				return ListSubordinateVolumesForVolume(
					volumeName)
			}},
			{name: "InconsistentReadSubordinateVolume", fn: func() Subquery { return InconsistentReadSubordinateVolume(subordinateVolumeName) }},
			{name: "ReadSubordinateVolume", fn: func() Subquery { return ReadSubordinateVolume(subordinateVolumeName) }},
			{name: "UpsertSubordinateVolume", fn: func() Subquery { return UpsertSubordinateVolume(subordinateVolumeName, volumeName) }},
			{name: "DeleteSubordinateVolume", fn: func() Subquery { return DeleteSubordinateVolume(subordinateVolumeName) }},
		})
	}

	return generators
}

type queryGenerator struct {
	name string
	fn   func() Subquery
}

// Update the generateRandomQuery function to ensure valid queries
func generateRandomQuery(generators []queryGenerator, maxQueries int) []Subquery {
	// Track which resource types have been used for each operation type
	usedResources := make(map[resource]map[operation]bool)

	numQueries := rand.Intn(maxQueries) + 1
	queries := make([]Subquery, 0, numQueries)

	// Shuffle generators to get random order
	shuffled := make([]queryGenerator, len(generators))
	copy(shuffled, generators)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for i := 0; i < numQueries && i < len(shuffled); i++ {
		subquery := shuffled[i].fn()

		// Check if this resource+operation combination is already used
		if usedResources[subquery.res] == nil {
			usedResources[subquery.res] = make(map[operation]bool)
		}

		// Skip if we already have this type of operation for this resource
		if usedResources[subquery.res][subquery.op] {
			continue
		}

		// Skip if we already have any read/update/delete ops for this resource
		if subquery.op == inconsistentRead || subquery.op == read || subquery.op == upsert || subquery.op == del {
			if usedResources[subquery.res][inconsistentRead] ||
				usedResources[subquery.res][read] ||
				usedResources[subquery.res][upsert] ||
				usedResources[subquery.res][del] {
				continue
			}
		}

		usedResources[subquery.res][subquery.op] = true
		queries = append(queries, subquery)
	}

	return queries
}

// TestLockNeverDeadlocks runs multiple concurrent Lock operations with random queries to ensure no deadlocks occur
// within or across Lock calls.
func TestLockNeverDeadlocks(t *testing.T) {
	testCases := []struct {
		name                  string
		numQueries            int
		maxSubqueriesPerQuery int
	}{
		{name: "small queries", numQueries: 2, maxSubqueriesPerQuery: 3},
		{name: "medium queries", numQueries: 5, maxSubqueriesPerQuery: 5},
		{name: "large queries", numQueries: 10, maxSubqueriesPerQuery: 10},
		{name: "many small queries", numQueries: 20, maxSubqueriesPerQuery: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up test data to avoid nil pointer issues
			setupTestData(t, 5)

			// Run multiple iterations with random query combinations
			count := 1000
			queryGenerators := createQueryGenerators(5)
			wg := sync.WaitGroup{}
			wg.Add(count)
			for i := 0; i < count; i++ {
				go func() {
					defer wg.Done()
					// Create random queries
					queries := make([][]Subquery, tc.numQueries)
					for i := 0; i < tc.numQueries; i++ {
						queries[i] = generateRandomQuery(queryGenerators[rand.Intn(len(queryGenerators))], tc.maxSubqueriesPerQuery)
					}
					results, unlock, err := Lock(context.Background(), queries...)
					defer unlock()
					time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond) // Simulate some processing time
					for _, operation := range []string{"Upsert", "Delete"} {
						doOperation(t, operation, results)
					}
					// Lock completed successfully
					assert.NoError(t, err, "Lock should not return an error for iteration %d", i)
					assert.Len(t, results, tc.numQueries, "should return correct number of results")
				}()
			}

			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				// Completed successfully
			case <-time.After(1 * time.Minute): // The maximum time to wait is count * 50ms, so 1 minute is safe
				t.Fatalf("Deadlock detected")
			}

			// Clean up test data
			cleanupTestData()
		})
	}
}

func TestLockWithDependencyChains(t *testing.T) {
	// Test specific patterns that might cause deadlocks based on the schema dependencies
	testCases := []struct {
		name      string
		querySets [][]Subquery
	}{
		{
			name: "volume and backend circular dependency",
			querySets: [][]Subquery{
				{
					UpsertVolume("vol1", "backend1"),
					DeleteBackend("backend1"),
				},
				{
					UpsertBackend("backend1", "backend1", ""),
					DeleteVolume("vol1"),
				},
			},
		},
		{
			name: "snapshot-volume-backend chain",
			querySets: [][]Subquery{
				{
					UpsertSnapshot("vol1", "snap1"),
					DeleteVolume("vol1"),
				},
				{
					UpsertVolume("vol1", "backend1"),
					DeleteSnapshot("snap1"),
				},
				{
					UpsertBackend("backend1", "backend1", ""),
					ListSnapshots(),
				},
			},
		},
		{
			name: "volume publication with both dependencies",
			querySets: [][]Subquery{
				{
					UpsertVolumePublication("vol1", "node1"),
					DeleteNode("node1"),
				},
				{
					UpsertNode("node1"),
					DeleteVolume("vol1"),
				},
				{
					UpsertVolume("vol1", "backend1"),
					ListVolumePublications(),
				},
			},
		},
		{
			name: "subordinate volume complex chain",
			querySets: [][]Subquery{
				{
					UpsertSubordinateVolume("subvol1", "vol1"),
					DeleteVolume("vol1"),
				},
				{
					UpsertVolume("vol1", "backend1"),
					DeleteBackend("backend1"),
				},
				{
					UpsertBackend("backend1", "backend1", ""),
					ListSubordinateVolumes(),
				},
			},
		},
		{
			name: "all resources mixed operations",
			querySets: [][]Subquery{
				{
					ListBackends(),
					UpsertVolume("vol1", "backend1"),
					UpsertSnapshot("vol1", "snap1"),
					UpsertNode("node1"),
				},
				{
					DeleteSnapshot("snap1"),
					UpsertStorageClass("sc1"),
					UpsertVolumePublication("vol1", "node1"),
					ListVolumes(),
				},
				{
					UpsertSubordinateVolume("subvol1", "vol1"),
					DeleteNode("node1"),
					DeleteStorageClass("sc1"),
					ReadBackend("backend1"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up test data
			setupTestData(t, 5)

			// Execute Lock with timeout
			done := make(chan struct{})
			var results []Result
			var unlock func()
			var err error

			go func() {
				results, unlock, err = Lock(context.Background(), tc.querySets...)
				time.Sleep(10 * time.Millisecond) // Simulate some processing time
				for _, operation := range []string{"Upsert", "Delete"} {
					doOperation(t, operation, results)
				}
				unlock()
				close(done)
			}()

			select {
			case <-done:
				// Lock completed successfully
				assert.NoError(t, err, "Lock should not return an error")
				assert.Len(t, results, len(tc.querySets), "should return correct number of results")
			case <-time.After(30 * time.Second):
				t.Fatalf("Deadlock detected for test case: %s", tc.name)
			}

			// Clean up test data
			cleanupTestData()
		})
	}
}

func doOperation(t *testing.T, operation string, results []Result) {
	t.Helper()
	for _, r := range results {
		v := reflect.ValueOf(r)
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.Kind() == reflect.Struct {
				upsertField := field.FieldByName(operation)
				readField := field.FieldByName("Read")
				if upsertField.IsValid() && !upsertField.IsNil() && readField.IsValid() && !readField.IsNil() {
					switch upsertField.Type().NumIn() {
					case 0:
						upsertField.Call(nil)
					case 1:
						upsertField.Call([]reflect.Value{readField})
					}
				}
			}
		}
	}
}

// Helper functions
func setupTestData(t *testing.T, numIds int) {
	// Create mock controller for backends
	mockCtrl := gomock.NewController(t)

	for i := 0; i < numIds; i++ {
		backendName := fmt.Sprintf("backend%d", i+1)
		volumeName := fmt.Sprintf("volume%d", i+1)
		snapshotName := fmt.Sprintf("snapshot%d", i+1)
		nodeName := fmt.Sprintf("node%d", i+1)
		storageClassName := fmt.Sprintf("sc%d", i+1)
		subordinateVolumeName := fmt.Sprintf("subvol%d", i+1)

		// Add some test data to avoid nil pointer dereferences
		backends.lock()
		backends.data[backendName] = getMockBackendWithMap(mockCtrl, map[string]string{
			"name":       backendName,
			"driverName": "test-driver",
			"uuid":       backendName,
			"uniqueKey":  backendName,
			"state":      "online",
		})
		backends.key.data[backendName] = backendName
		backends.unlock()

		volumes.lock()
		volumes.data[volumeName] = &storage.Volume{
			Config: &storage.VolumeConfig{
				Name:         volumeName,
				InternalName: volumeName,
				Size:         "1Gi",
			},
			BackendUUID: backendName,
			State:       storage.VolumeStateOnline,
		}
		volumes.key.data[volumeName] = volumeName
		volumes.unlock()

		nodes.lock()
		nodes.data[nodeName] = &models.Node{
			Name: nodeName,
			IQN:  "iqn.test.node1",
		}
		nodes.unlock()

		storageClasses.lock()
		storageClasses.data[storageClassName] = storageclass.New(&storageclass.Config{
			Name: storageClassName,
		})
		storageClasses.unlock()

		snapshots.lock()
		snapshots.data[snapshotName] = &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Name:       snapshotName,
				VolumeName: volumeName,
			},
		}
		snapshots.unlock()

		subordinateVolumes.lock()
		subordinateVolumes.data[subordinateVolumeName] = &storage.Volume{
			Config: &storage.VolumeConfig{
				Name:              subordinateVolumeName,
				InternalName:      subordinateVolumeName,
				ShareSourceVolume: volumeName,
				Size:              "1Gi",
			},
			BackendUUID: backendName,
			State:       storage.VolumeStateOnline,
		}
		subordinateVolumes.unlock()

		volumePublications.lock()
		volumePublications.data[fmt.Sprintf("%s.%s", volumeName, nodeName)] = &models.VolumePublication{
			VolumeName: volumeName,
			NodeName:   nodeName,
		}
		volumePublications.unlock()
	}
}

func cleanupTestData() {
	// Clean up all test data
	backends.lock()
	backends.data = make(map[string]SmartCopier)
	backends.key.data = make(map[string]string)
	backends.unlock()

	volumes.lock()
	volumes.data = make(map[string]SmartCopier)
	volumes.key.data = make(map[string]string)
	volumes.unlock()

	snapshots.lock()
	snapshots.data = make(map[string]SmartCopier)
	snapshots.unlock()

	nodes.lock()
	nodes.data = make(map[string]SmartCopier)
	nodes.unlock()

	storageClasses.lock()
	storageClasses.data = make(map[string]SmartCopier)
	storageClasses.unlock()

	subordinateVolumes.lock()
	subordinateVolumes.data = make(map[string]SmartCopier)
	subordinateVolumes.unlock()

	volumePublications.lock()
	volumePublications.data = make(map[string]SmartCopier)
	volumePublications.unlock()
}

func TestLockCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancel()
	_, unlocker, err := Lock(ctx, Query(UpsertNode("node1")))
	defer unlocker()
	assert.ErrorContains(t, err, "context deadline exceeded")
}
