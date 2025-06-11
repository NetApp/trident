package concurrent_cache

import (
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage"
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
			results, unlocker, err := Lock(makeQueries()...)
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

// WIP
// func TestLock_Deadlock(t *testing.T) {
// 	err := quick.Check(func(querySet [][]DeadlockQueryGenerator) bool {
// 		initCaches()
// 		defer cleanCaches()
// 		wg := sync.WaitGroup{}
// 		wg.Add(len(querySet))
//
// 		for _, queryGen := range querySet {
// 			queries := make([][]Subquery, 0, len(queryGen))
// 			for _, q := range queryGen {
// 				queries = append(queries, q)
// 			}
// 			go func() {
// 				defer wg.Done()
// 				results, unlocker, err := Lock(queries...)
// 				defer unlocker()
// 				assert.NoError(t, err)
// 				assert.NotEmpty(t, results)
// 				time.Sleep(1 * time.Second)
// 			}()
// 		}
// 		return true
// 	}, nil)
// 	assert.NoError(t, err)
// }

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

var existingIDs = []string{
	"zero",
	"one",
	"two",
	"three",
	"four",
}

type DeadlockQueryGenerator []Subquery

// Generate generates a random query for testing deadlock detection.
// Each list operation has a 50% chance of being selected.
// Each resource type has 20% chance of being selected, and then each op has an equal chance of being selected.
// To detect deadlocks we need to test multiple queries using the same IDs, so for any subquery there is a 50% chance of
// using existing IDs and a 50% chance of using new IDs.
func (d DeadlockQueryGenerator) Generate(rand *rand.Rand, _ int) reflect.Value {
	// max length is number of resources * 2 (lists + CRUD for each resource)
	subqueries := make([]Subquery, 0, resourceCount*2)
	for i := 0; i < resourceCount; i++ {
		if rand.Intn(2) == 0 {
			var s Subquery
			switch resource(i) {
			case node:
				s = ListNodes()
			case storageClass:
				s = ListStorageClasses()
			case backend:
				s = ListBackends()
			case volume:
				s = ListVolumes()
			case subordinateVolume:
				s = ListSubordinateVolumes()
			case volumePublication:
				s = ListVolumePublications()
			case snapshot:
				s = ListSnapshots()
			}
			subqueries = append(subqueries, s)
		}
		if rand.Intn(5) == 0 {
			op := operation(rand.Intn(4) + 1)
			useExistingID := rand.Intn(2) == 0
			subqueries = append(subqueries, crudSubquery(rand, resource(i), op, useExistingID))
		}
	}
	rand.Shuffle(len(subqueries), func(i, j int) {
		subqueries[i], subqueries[j] = subqueries[j], subqueries[i]
	})
	return reflect.ValueOf(subqueries)
}

func crudSubquery(rand *rand.Rand, r resource, o operation, useExistingID bool) Subquery {
	var id, id2 string
	if useExistingID {
		id = existingIDs[rand.Intn(len(existingIDs))]
		id2 = existingIDs[rand.Intn(len(existingIDs))]
	} else {
		id = randomString(rand, 20)
		id2 = randomString(rand, 20)
	}
	switch r {
	case node:
		switch o {
		case read:
			return ReadNode(id)
		case upsert:
			return UpsertNode(id)
		case del:
			return DeleteNode(id)
		}
	case storageClass:
		switch o {
		case read:
			return ReadStorageClass(id)
		case upsert:
			return UpsertStorageClass(id)
		case del:
			return DeleteStorageClass(id)
		}
	case backend:
		switch o {
		case read:
			return ReadBackend(id)
		case upsert:
			return UpsertBackend(id, id2, id2)
		case del:
			return DeleteBackend(id)
		}
	case volume:
		switch o {
		case read:
			return ReadVolume(id)
		case upsert:
			return UpsertVolume(id, id2)
		case del:
			return DeleteVolume(id)
		}
	case subordinateVolume:
		switch o {
		case read:
			return ReadSubordinateVolume(id)
		case upsert:
			return UpsertSubordinateVolume(id, id2)
		case del:
			return DeleteSubordinateVolume(id)
		}
	case volumePublication:
		switch o {
		case read:
			return ReadVolumePublication(id, id2)
		case upsert:
			return UpsertVolumePublication(id, id2)
		case del:
			return DeleteVolumePublication(id, id2)
		}
	case snapshot:
		switch o {
		case read:
			return ReadSnapshot(id)
		case upsert:
			return UpsertSnapshot(id, id2)
		case del:
			return DeleteSnapshot(id)
		}
	}
	return Subquery{}
}

func randomString(rand *rand.Rand, size int) string {
	const maxRune = 0x10ffff
	s := make([]rune, size)
	for i := 0; i < len(s); i++ {
		s[i] = rune(rand.Intn(maxRune))
	}
	return string(s)
}
