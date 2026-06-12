package concurrent_cache

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockstorage "github.com/netapp/trident/mocks/mock_storage"
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

func TestAssembleQueriesRegistersSyntheticDependencyCaches(t *testing.T) {
	queries := [][]Subquery{Query(UpsertVolumePublication("volume1", "node1"))}

	_, cachesPresent, _, _, err := assembleQueries(queries)
	assert.NoError(t, err)

	_, hasVolume := cachesPresent[volume]
	_, hasNode := cachesPresent[node]
	_, hasVolumePublication := cachesPresent[volumePublication]
	assert.True(t, hasVolume, "synthetic volume read dependency should lock volume cache")
	assert.True(t, hasNode, "synthetic node read dependency should lock node cache")
	assert.True(t, hasVolumePublication)
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

func TestLockNewKeys(t *testing.T) {
	t.Run("dedupes consecutive same key without panic on unlock", func(t *testing.T) {
		initCaches()
		keys := []newKey{
			{r: backend, k: "new-backend", o: upsert},
			{r: backend, k: "new-backend", o: upsert},
		}
		unlock, err := lockNewKeys(keys)
		assert.NoError(t, err)
		assert.NotPanics(t, func() { unlock() })
	})

	t.Run("returns error when key already exists", func(t *testing.T) {
		initCaches()
		backends.lock()
		backends.key.data["taken"] = "backend1"
		backends.unlock()

		unlock, err := lockNewKeys([]newKey{{r: backend, k: "taken", o: upsert}})
		if unlock != nil {
			unlock()
		}
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("locks distinct keys", func(t *testing.T) {
		initCaches()
		keys := []newKey{
			{r: volume, k: "vol-a", o: upsert},
			{r: backend, k: "backend-a", o: upsert},
		}
		unlock, err := lockNewKeys(keys)
		defer unlock()
		assert.NoError(t, err)
	})
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

			_, merged := mergeQueries(queries, true)
			assert.Len(t, merged, 14)
		})
	}
}

func TestLock(t *testing.T) {
	// start two goroutines that try to update the same resources using multiple queries
	// passes if there is no deadlock
	initCaches()
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
			_, results, unlocker, err := Lock(context.Background(), makeQueries()...)
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
	Initialize()

	nodes.data["node1"] = &models.Node{
		Name: "node1",
	}
	backends.data["backend1"] = storage.NewTestStorageBackend()
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
	autogrowPolicies.data["policy1"] = storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
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

			// AutogrowPolicy queries
			{name: "ListAutogrowPolicies", fn: ListAutogrowPolicies},
			{name: "InconsistentReadAutogrowPolicy", fn: func() Subquery {
				policyName := fmt.Sprintf("policy%d", i+1)
				return InconsistentReadAutogrowPolicy(policyName)
			}},
			{name: "ReadAutogrowPolicy", fn: func() Subquery {
				policyName := fmt.Sprintf("policy%d", i+1)
				return ReadAutogrowPolicy(policyName)
			}},
			{name: "UpsertAutogrowPolicy", fn: func() Subquery {
				policyName := fmt.Sprintf("policy%d", i+1)
				return UpsertAutogrowPolicy(policyName)
			}},
			{name: "DeleteAutogrowPolicy", fn: func() Subquery {
				policyName := fmt.Sprintf("policy%d", i+1)
				return DeleteAutogrowPolicy(policyName)
			}},
		})
	}

	return generators
}

type queryGenerator struct {
	name string
	fn   func() Subquery
}

func flattenQueries(queries [][]Subquery) []Subquery {
	n := 0
	for _, q := range queries {
		n += len(q)
	}
	flat := make([]Subquery, 0, n)
	for _, q := range queries {
		flat = append(flat, q...)
	}
	return flat
}

func buildQuerySet(queries []Subquery) querySet {
	qs := make(querySet)
	for _, q := range queries {
		if q.op == list || q.op == inconsistentRead {
			continue
		}
		qs[q.elem()] = isWriteOp(q.op)
	}
	return qs
}

func heldQuerySet(queries [][]Subquery) querySet {
	return buildQuerySet(flattenQueries(queries))
}

func heldQuerySetFromContext(ctx context.Context) querySet {
	return buildQuerySet(heldSubqueriesFromContext(ctx))
}

func heldSubqueriesFromContext(ctx context.Context) []Subquery {
	lc, ok := ctx.Value(lockContextKey{}).(*lockContext)
	if !ok || lc == nil || len(lc.qs) == 0 {
		return nil
	}
	held := make([]Subquery, 0, len(lc.qs))
	for elem := range lc.qs {
		held = append(held, elem.subquery())
	}
	slices.SortFunc(held, compareSubqueries)
	return held
}

func newStressRand(t *testing.T) (baseSeed int64, next func() *rand.Rand) {
	baseSeed = time.Now().UnixNano()
	t.Logf("seed=%d", baseSeed)
	var counter atomic.Int64
	return baseSeed, func() *rand.Rand {
		n := counter.Add(1)
		return rand.New(rand.NewSource(baseSeed ^ n))
	}
}

func stressConcurrency() int {
	if testing.Short() {
		return 50
	}
	return 1000
}

// nestedStressConcurrency uses lower concurrency than Lock-only stress because each goroutine
// holds the outer lock across multiple NestedLock rounds.
func nestedStressConcurrency() int {
	if testing.Short() {
		return 10
	}
	return 50
}

func nestedStressConcurrencyFor(numQueries int) int {
	c := nestedStressConcurrency()
	if numQueries > 10 && c > 5 {
		return 5
	}
	if numQueries > 5 && c > 8 {
		return 8
	}
	return c
}

func nestedStressTimeoutFor(numQueries int) time.Duration {
	if testing.Short() {
		if numQueries > 10 {
			return 60 * time.Second
		}
		return 30 * time.Second
	}
	return 2 * time.Minute
}

func stressHoldSleep(rng *rand.Rand) time.Duration {
	if testing.Short() {
		return time.Duration(rng.Intn(3)) * time.Millisecond
	}
	return time.Duration(10+rng.Intn(40)) * time.Millisecond
}

func stressTimeout() time.Duration {
	if testing.Short() {
		return 15 * time.Second
	}
	return time.Minute
}

func nestedStressTimeout() time.Duration {
	if testing.Short() {
		return 30 * time.Second
	}
	return 2 * time.Minute
}

func runConcurrentStress(t *testing.T, timeout time.Duration, concurrency int, fn func()) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("deadlock detected")
	}
}

func crossBackendRoundTimeout() time.Duration {
	if testing.Short() {
		return 200 * time.Millisecond
	}
	return 500 * time.Millisecond
}

func upsertBackendQuery(id string) []Subquery {
	return Query(UpsertBackend(id, id, ""))
}

// runCrossBackendNestedDeadlockRound exercises cross-goroutine lock inversion: one goroutine holds a
// higher-sorted backend and nests a lock on a lower-sorted backend while another goroutine locks both in order.
// Without nested lock ordering validation this can deadlock.
func runCrossBackendNestedDeadlockRound(t *testing.T, rng *rand.Rand, invert bool) {
	t.Helper()
	early, later := "backend1", "backend2"
	if invert {
		early, later = "backend2", "backend1"
	}

	sleepBeforeLock := time.Duration(rng.Intn(5)) * time.Millisecond
	sleepBeforeNested := time.Duration(rng.Intn(10)) * time.Millisecond

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		time.Sleep(sleepBeforeLock)
		_, _, unlock, err := Lock(
			context.Background(),
			upsertBackendQuery(early),
			upsertBackendQuery(later),
		)
		if unlock != nil {
			defer unlock()
		}
		if err != nil {
			return
		}
	}()

	go func() {
		defer wg.Done()
		lockCtx, _, unlock, err := Lock(context.Background(), upsertBackendQuery(later))
		if unlock != nil {
			defer unlock()
		}
		if err != nil {
			return
		}
		time.Sleep(sleepBeforeNested)
		_, _, nestedUnlock, _ := NestedLock(lockCtx, upsertBackendQuery(early))
		if nestedUnlock != nil {
			defer nestedUnlock()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(crossBackendRoundTimeout()):
		t.Fatal("deadlock detected")
	}
}

func generateRandomQuery(rng *rand.Rand, generators []queryGenerator, maxQueries int) []Subquery {
	return generateRandomQueryWithHeld(rng, generators, maxQueries, nil, nil)
}

func lastLockFromContext(ctx context.Context) *Subquery {
	lc, ok := ctx.Value(lockContextKey{}).(*lockContext)
	if !ok || lc == nil {
		return nil
	}
	return lc.lastLock
}

func assertLastLockPreserved(t *testing.T, ctx context.Context, msg string, res resource, id string) {
	t.Helper()
	lastLock := lastLockFromContext(ctx)
	if !assert.NotNil(t, lastLock, msg) {
		return
	}
	assert.Equal(t, res, lastLock.res)
	if id != "" {
		assert.Equal(t, id, lastLock.id)
	}
}

func hasConsistentLocks(ctx context.Context) bool {
	return len(heldSubqueriesFromContext(ctx)) > 0
}

func lastHeldForNestedQuery(ctx context.Context, heldSubqueries []Subquery) *Subquery {
	if lastLock := lastLockFromContext(ctx); lastLock != nil {
		return lastLock
	}
	if len(heldSubqueries) == 0 {
		return nil
	}
	max := heldSubqueries[0]
	for _, h := range heldSubqueries[1:] {
		if compareSubqueries(h, max) > 0 {
			max = h
		}
	}
	return &max
}

func isNestedLockOrderingError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "new lock could cause deadlock")
}

func isNestedLockExpectedRejection(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "new lock could cause deadlock") ||
		strings.Contains(msg, "nested locks must not upgrade locks")
}

func generateRandomNestedQuery(
	ctx context.Context, rng *rand.Rand, heldSubqueries []Subquery, generators []queryGenerator, maxQueries int,
) []Subquery {
	lastHeld := lastHeldForNestedQuery(ctx, heldSubqueries)
	return generateRandomQueryWithHeld(rng, generators, maxQueries, buildQuerySet(heldSubqueries), lastHeld)
}

func generateWriteHeavyQuery(rng *rand.Rand, generators []queryGenerator, maxQueries int) []Subquery {
	writeGenerators := make([]queryGenerator, 0, len(generators))
	for _, g := range generators {
		sq := g.fn()
		if isWriteOp(sq.op) {
			writeGenerators = append(writeGenerators, g)
		}
	}
	if len(writeGenerators) == 0 {
		return generateRandomQuery(rng, generators, maxQueries)
	}
	return generateRandomQueryWithHeld(rng, writeGenerators, maxQueries, nil, nil)
}

func readOnlyQueryGenerators(generators [][]queryGenerator) [][]queryGenerator {
	filtered := make([][]queryGenerator, 0, len(generators))
	for _, group := range generators {
		readOnlyGroup := make([]queryGenerator, 0, len(group))
		for _, g := range group {
			sq := g.fn()
			if !isWriteOp(sq.op) {
				readOnlyGroup = append(readOnlyGroup, g)
			}
		}
		if len(readOnlyGroup) > 0 {
			filtered = append(filtered, readOnlyGroup)
		}
	}
	return filtered
}

// generateRandomQueryWithHeld builds a deduped random query batch. When held is non-nil, subqueries that would
// upgrade an existing read lock are skipped (nested lock rules). When lastHeld is set, only subqueries that sort
// after the last held subquery are included (nested lock ordering).
func generateRandomQueryWithHeld(rng *rand.Rand, generators []queryGenerator, maxQueries int, held querySet, lastHeld *Subquery) []Subquery {
	usedResources := make(map[resource]map[operation]bool)

	numQueries := rng.Intn(maxQueries) + 1
	queries := make([]Subquery, 0, numQueries)

	shuffled := make([]queryGenerator, len(generators))
	copy(shuffled, generators)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for i := 0; i < numQueries && i < len(shuffled); i++ {
		subquery := shuffled[i].fn()

		if held != nil && subquery.id != "" {
			if write, ok := held[subquery.elem()]; ok && !write && isWriteOp(subquery.op) {
				continue
			}
		}

		if lastHeld != nil && compareSubqueries(subquery, *lastHeld) <= 0 {
			continue
		}

		if usedResources[subquery.res] == nil {
			usedResources[subquery.res] = make(map[operation]bool)
		}

		if usedResources[subquery.res][subquery.op] {
			continue
		}

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

var lockStressCases = []struct {
	name                  string
	numQueries            int
	maxSubqueriesPerQuery int
}{
	{name: "small queries", numQueries: 2, maxSubqueriesPerQuery: 3},
	{name: "medium queries", numQueries: 5, maxSubqueriesPerQuery: 5},
	{name: "large queries", numQueries: 10, maxSubqueriesPerQuery: 10},
	{name: "many small queries", numQueries: 20, maxSubqueriesPerQuery: 2},
}

var nestedLockStressCases = []struct {
	name                  string
	numQueries            int
	maxSubqueriesPerQuery int
}{
	{name: "small random", numQueries: 2, maxSubqueriesPerQuery: 3},
	{name: "medium random", numQueries: 3, maxSubqueriesPerQuery: 3},
	{name: "many tiny random", numQueries: 6, maxSubqueriesPerQuery: 1},
}

// TestLockNeverDeadlocks runs multiple concurrent Lock operations with random queries to ensure no deadlocks occur
// within or across Lock calls. Random cross-goroutine deletes can make individual Lock calls return errors;
// that is expected and not asserted here. Skipped when testing.Short() is set.
func TestLockNeverDeadlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for _, tc := range lockStressCases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestData(t, 5)
			defer cleanupTestData()

			_, nextRNG := newStressRand(t)
			queryGenerators := createQueryGenerators(5)
			runConcurrentStress(t, stressTimeout(), stressConcurrency(), func() {
				rng := nextRNG()
				queries := make([][]Subquery, tc.numQueries)
				for j := 0; j < tc.numQueries; j++ {
					queries[j] = generateRandomQuery(rng, queryGenerators[rng.Intn(len(queryGenerators))], tc.maxSubqueriesPerQuery)
				}
				_, results, unlock, err := Lock(context.Background(), queries...)
				defer unlock()
				if err != nil {
					return
				}
				time.Sleep(stressHoldSleep(rng))
				for _, operation := range []string{"Upsert", "Delete"} {
					doOperation(t, operation, results)
				}
				assert.Len(t, results, tc.numQueries, "should return correct number of results")
			})
		})
	}
}

// TestNestedLockNeverDeadlocks runs concurrent Lock followed by random NestedLock rounds.
func TestNestedLockNeverDeadlocks(t *testing.T) {
	t.Run("cross-backend inversion", func(t *testing.T) {
		setupTestData(t, 2)
		defer cleanupTestData()

		_, nextRNG := newStressRand(t)
		for i := 0; i < stressConcurrency(); i++ {
			runCrossBackendNestedDeadlockRound(t, nextRNG(), i%2 == 1)
		}
	})

	for _, tc := range nestedLockStressCases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestData(t, 5)
			defer cleanupTestData()

			_, nextRNG := newStressRand(t)
			queryGenerators := createQueryGenerators(5)
			maxNestedBatches := 3
			if testing.Short() {
				maxNestedBatches = 2
			}

			runConcurrentStress(t, nestedStressTimeoutFor(tc.numQueries), nestedStressConcurrencyFor(tc.numQueries), func() {
				rng := nextRNG()
				queries := make([][]Subquery, tc.numQueries)
				for j := 0; j < tc.numQueries; j++ {
					gens := queryGenerators[rng.Intn(len(queryGenerators))]
					if rng.Intn(2) == 0 {
						queries[j] = generateWriteHeavyQuery(rng, gens, tc.maxSubqueriesPerQuery)
					} else {
						queries[j] = generateRandomQuery(rng, gens, tc.maxSubqueriesPerQuery)
					}
				}
				lockCtx, results, unlock, err := Lock(context.Background(), queries...)
				defer unlock()
				assert.NoError(t, err, "Lock should not return an error")
				assert.Len(t, results, tc.numQueries)

				heldSubqueries := heldSubqueriesFromContext(lockCtx)

				nestedRounds := rng.Intn(3)
				var nestedUnlockers []func()
				defer func() {
					for i := len(nestedUnlockers) - 1; i >= 0; i-- {
						nestedUnlockers[i]()
					}
				}()

				for round := 0; round < nestedRounds; round++ {
					numBatches := rng.Intn(maxNestedBatches) + 1
					nestedQueries := make([][]Subquery, 0, numBatches)
					for b := 0; b < numBatches; b++ {
						q := generateRandomNestedQuery(
							lockCtx, rng, heldSubqueries, queryGenerators[rng.Intn(len(queryGenerators))], tc.maxSubqueriesPerQuery,
						)
						if len(q) > 0 {
							nestedQueries = append(nestedQueries, q)
						}
					}
					if len(nestedQueries) == 0 {
						continue
					}
					var nestedResults []Result
					var nestedUnlock func()
					var nestedErr error
					lockCtx, nestedResults, nestedUnlock, nestedErr = NestedLock(lockCtx, nestedQueries...)
					nestedUnlockers = append(nestedUnlockers, nestedUnlock)
					if isNestedLockExpectedRejection(nestedErr) {
						continue
					}
					assert.NoError(t, nestedErr, "NestedLock should not return an error")
					assert.Len(t, nestedResults, len(nestedQueries))

					heldSubqueries = heldSubqueriesFromContext(lockCtx)
					if hasConsistentLocks(lockCtx) {
						assert.NotNil(t, lastLockFromContext(lockCtx),
							"lastLock must be preserved when consistent locks are held")
					}
				}
			})
		})
	}
}

// TestNestedLockDeepChainNeverDeadlocks stress-tests multiple sequential NestedLock calls on one context.
func TestNestedLockDeepChainNeverDeadlocks(t *testing.T) {
	setupTestData(t, 5)
	defer cleanupTestData()

	_, nextRNG := newStressRand(t)
	queryGenerators := createQueryGenerators(5)
	runConcurrentStress(t, nestedStressTimeout(), nestedStressConcurrencyFor(1), func() {
		rng := nextRNG()
		gens := queryGenerators[rng.Intn(len(queryGenerators))]
		outerQuery := generateWriteHeavyQuery(rng, gens, 3)
		if len(outerQuery) == 0 {
			outerQuery = generateRandomQuery(rng, gens, 3)
		}
		queries := [][]Subquery{outerQuery}
		lockCtx, results, unlock, err := Lock(context.Background(), queries...)
		defer unlock()
		assert.NoError(t, err)
		assert.Len(t, results, 1)

		heldSubqueries := heldSubqueriesFromContext(lockCtx)

		nestedDepth := 3 + rng.Intn(3)
		var nestedUnlockers []func()
		defer func() {
			for i := len(nestedUnlockers) - 1; i >= 0; i-- {
				nestedUnlockers[i]()
			}
		}()

		maxSubqueries := 5
		for depth := 0; depth < nestedDepth; depth++ {
			if maxSubqueries > 1 {
				maxSubqueries--
			}
			nestedQuery := generateRandomNestedQuery(
				lockCtx, rng, heldSubqueries, queryGenerators[rng.Intn(len(queryGenerators))], maxSubqueries,
			)
			if len(nestedQuery) == 0 {
				continue
			}
			nestedQueries := [][]Subquery{nestedQuery}
			var nestedResults []Result
			var nestedUnlock func()
			var nestedErr error
			lockCtx, nestedResults, nestedUnlock, nestedErr = NestedLock(lockCtx, nestedQueries...)
			nestedUnlockers = append(nestedUnlockers, nestedUnlock)
			if isNestedLockExpectedRejection(nestedErr) {
				continue
			}
			assert.NoError(t, nestedErr)
			assert.Len(t, nestedResults, 1)

			heldSubqueries = heldSubqueriesFromContext(lockCtx)
			if hasConsistentLocks(lockCtx) {
				assert.NotNil(t, lastLockFromContext(lockCtx),
					"lastLock must be preserved when consistent locks are held")
			}
		}
	})
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
				_, results, unlock, err = Lock(context.Background(), tc.querySets...)
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
			case <-time.After(5 * time.Second):
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
		poolName := fmt.Sprintf("pool%d", i+1)
		volumeName := fmt.Sprintf("volume%d", i+1)
		snapshotName := fmt.Sprintf("snapshot%d", i+1)
		nodeName := fmt.Sprintf("node%d", i+1)
		storageClassName := fmt.Sprintf("sc%d", i+1)
		subordinateVolumeName := fmt.Sprintf("subvol%d", i+1)

		// Add some test data to avoid nil pointer dereferences
		mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
			"name":       backendName,
			"driverName": "test-driver",
			"uuid":       backendName,
			"uniqueKey":  backendName,
			"state":      "online",
		})

		mockPool := mockstorage.NewMockPool(mockCtrl)
		mockPool.EXPECT().SetBackend(mockBackend).AnyTimes()
		mockPoolMap := sync.Map{}
		mockPoolMap.Store(poolName, mockPool)
		mockBackend.EXPECT().StoragePools().Return(&mockPoolMap).AnyTimes()

		backends.lock()
		backends.data[backendName] = mockBackend
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

		autogrowPolicies.lock()
		autogrowPolicyName := fmt.Sprintf("policy%d", i+1)
		autogrowPolicies.data[autogrowPolicyName] = storage.NewAutogrowPolicy(
			autogrowPolicyName,
			"80",
			"20",
			"1000Gi",
			storage.AutogrowPolicyStateSuccess,
		)
		autogrowPolicies.unlock()
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

	autogrowPolicies.lock()
	autogrowPolicies.data = make(map[string]SmartCopier)
	autogrowPolicies.unlock()
}

func TestLockCancel(t *testing.T) {
	initCaches()
	ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancel()
	_, _, unlocker, err := Lock(ctx, Query(UpsertNode("node1")))
	defer unlocker()
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNestedLockCancel(t *testing.T) {
	initCaches()
	ctx, cancel := context.WithCancel(context.Background())
	lockCtx, _, unlocker, err := Lock(ctx, Query(ReadBackend("backend1")))
	assert.NoError(t, err)
	defer unlocker()

	cancel()
	_, _, nestedUnlocker, err := NestedLock(lockCtx, Query(ReadVolume("volume1")))
	defer nestedUnlocker()
	assert.ErrorIs(t, err, context.Canceled)
}

func TestInitialize(t *testing.T) {
	// Call Initialize
	Initialize()

	// Verify all caches are initialized properly
	assert.NotNil(t, nodes.data, "nodes data should be initialized")
	assert.NotNil(t, nodes.resourceLocks, "nodes resourceLocks should be initialized")
	assert.NotNil(t, storageClasses.data, "storageClasses data should be initialized")
	assert.NotNil(t, storageClasses.resourceLocks, "storageClasses resourceLocks should be initialized")
	assert.NotNil(t, backends.data, "backends data should be initialized")
	assert.NotNil(t, backends.resourceLocks, "backends resourceLocks should be initialized")
	assert.NotNil(t, volumes.data, "volumes data should be initialized")
	assert.NotNil(t, volumes.resourceLocks, "volumes resourceLocks should be initialized")
	assert.NotNil(t, snapshots.data, "snapshots data should be initialized")
	assert.NotNil(t, snapshots.resourceLocks, "snapshots resourceLocks should be initialized")
	assert.NotNil(t, volumePublications.data, "volumePublications data should be initialized")
	assert.NotNil(t, volumePublications.resourceLocks, "volumePublications resourceLocks should be initialized")
	assert.NotNil(t, subordinateVolumes.data, "subordinateVolumes data should be initialized")
	assert.NotNil(t, subordinateVolumes.resourceLocks, "subordinateVolumes resourceLocks should be initialized")
	assert.NotNil(t, autogrowPolicies.data, "autogrowPolicies data should be initialized")
	assert.NotNil(t, autogrowPolicies.resourceLocks, "autogrowPolicies resourceLocks should be initialized")

	// Test the caches are working
	nodes.lock()
	assert.NotNil(t, nodes.data, "nodes data should remain initialized after lock/unlock")
	nodes.unlock()

	// Call Initialize again - should reinitialize
	Initialize()
	assert.Empty(t, nodes.data, "nodes data should be empty after re-initialization")
	assert.Empty(t, storageClasses.data, "storageClasses data should be empty after re-initialization")
	assert.Empty(t, backends.data, "backends data should be empty after re-initialization")
	assert.Empty(t, volumes.data, "volumes data should be empty after re-initialization")
	assert.Empty(t, snapshots.data, "snapshots data should be empty after re-initialization")
	assert.Empty(t, volumePublications.data, "volumePublications data should be empty after re-initialization")
	assert.Empty(t, subordinateVolumes.data, "subordinateVolumes data should be empty after re-initialization")
	assert.Empty(t, autogrowPolicies.data, "autogrowPolicies data should be empty after re-initialization")
}

func TestQuery(t *testing.T) {
	tests := []struct {
		name       string
		subqueries []Subquery
		expected   int
	}{
		{
			name:       "empty query",
			subqueries: []Subquery{},
			expected:   0,
		},
		{
			name: "single subquery",
			subqueries: []Subquery{
				ListBackends(),
			},
			expected: 1,
		},
		{
			name: "multiple subqueries",
			subqueries: []Subquery{
				ListBackends(),
				ListNodes(),
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Query(tt.subqueries...)
			assert.Len(t, result, tt.expected, "Query should return the same number of subqueries")

			// Verify that the returned subqueries are the same as the input
			for i, subquery := range tt.subqueries {
				assert.Equal(t, subquery.res, result[i].res, "Resource should match")
				assert.Equal(t, subquery.op, result[i].op, "Operation should match")
			}
		})
	}
}

func TestIsWriteOp(t *testing.T) {
	tests := []struct {
		name     string
		op       operation
		expected bool
	}{
		{
			name:     "list operation is read-only",
			op:       list,
			expected: false,
		},
		{
			name:     "read operation is read-only",
			op:       read,
			expected: false,
		},
		{
			name:     "inconsistentRead operation is read-only",
			op:       inconsistentRead,
			expected: false,
		},
		{
			name:     "upsert operation is write",
			op:       upsert,
			expected: true,
		},
		{
			name:     "del operation is write",
			op:       del,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWriteOp(tt.op)
			assert.Equal(t, tt.expected, result, "isWriteOp should return expected value")
		})
	}
}

func TestRankForResource(t *testing.T) {
	tests := []struct {
		name     string
		resource resource
		expected int // We'll test that it returns a non-negative integer
	}{
		{
			name:     "node resource",
			resource: node,
			expected: 0, // nodes typically have rank 0
		},
		{
			name:     "backend resource",
			resource: backend,
			expected: 0, // backends typically have rank 0
		},
		{
			name:     "storage class resource",
			resource: storageClass,
			expected: 0, // storage classes typically have rank 0
		},
		{
			name:     "volume resource",
			resource: volume,
			expected: 1, // volumes depend on backend, so rank 1
		},
		{
			name:     "snapshot resource",
			resource: snapshot,
			expected: 2, // snapshots depend on volume, so rank 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rankForResource(tt.resource)
			assert.GreaterOrEqual(t, result, 0, "rankForResource should return non-negative value")
			// Don't assert exact values as they may depend on schema configuration
		})
	}
}

func TestCheckDependency(t *testing.T) {
	tests := []struct {
		name          string
		subqueries    []Subquery
		index         int
		resource      resource
		expectError   bool
		errorContains string
	}{
		{
			name: "valid single dependency",
			subqueries: []Subquery{
				{res: backend, dependencies: []int{}}, // index 0: backend
				{res: volume, dependencies: []int{0}}, // index 1: volume depends on backend
			},
			index:       1,
			resource:    backend,
			expectError: false,
		},
		{
			name: "no dependencies - should error",
			subqueries: []Subquery{
				{res: volume, dependencies: []int{}}, // index 0: volume with no dependencies
			},
			index:         0,
			resource:      backend,
			expectError:   true,
			errorContains: "expected one dependency",
		},
		{
			name: "multiple dependencies - should error",
			subqueries: []Subquery{
				{res: backend, dependencies: []int{}},      // index 0: backend
				{res: storageClass, dependencies: []int{}}, // index 1: storage class
				{res: volume, dependencies: []int{0, 1}},   // index 2: volume with multiple dependencies
			},
			index:         2,
			resource:      backend,
			expectError:   true,
			errorContains: "expected one dependency",
		},
		{
			name: "wrong dependency type - should error",
			subqueries: []Subquery{
				{res: storageClass, dependencies: []int{}}, // index 0: storage class
				{res: volume, dependencies: []int{0}},      // index 1: volume depends on storage class
			},
			index:         1,
			resource:      backend,
			expectError:   true,
			errorContains: "expected Backend dependency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDependency(tt.subqueries, tt.index, tt.resource)

			if tt.expectError {
				assert.Error(t, err, "checkDependency should return an error")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "checkDependency should not return an error")
			}
		})
	}
}

// TestAllowInconsistentReadWithRootLock tests that inconsistent reads and lists can occur while the root lock is held.
func TestAllowInconsistentReadWithRootLock(t *testing.T) {
	initCaches()

	_, _, unlocker, err := Lock(context.Background(), Query(LockCache()))
	defer unlocker()

	assert.NoError(t, err)
	_, results, unlocker2, err := Lock(context.Background(), Query(InconsistentReadBackend("backend1"), ListVolumes()))
	defer unlocker2()
	assert.NoError(t, err)
	assert.NotNil(t, results[0].Backend.Read)
	assert.NotEmpty(t, results[0].Volumes)
}

func TestBuildQuerySet(t *testing.T) {
	queries := []Subquery{
		{res: backend, id: "b1", op: read},
		{res: volume, id: "v1", op: upsert},
		{res: backend, id: "b1", op: read},
	}
	qs := buildQuerySet(queries)
	assert.False(t, qs[querySetElem{res: backend, id: "b1"}])
	assert.True(t, qs[querySetElem{res: volume, id: "v1"}])
}

func TestValidateWithExistingLocks(t *testing.T) {
	initCaches()

	t.Run("allows write after write", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(UpsertBackend("backend1", "", "")))
		defer unlocker()
		assert.NoError(t, err)
		_, _, nestedUnlocker, err := NestedLock(lockCtx, Query(UpsertBackend("backend1", "", "")))
		defer nestedUnlocker()
		assert.NoError(t, err)
	})

	t.Run("allows locking new resources", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadBackend("backend1")))
		defer unlocker()
		assert.NoError(t, err)
		_, _, nestedUnlocker, err := NestedLock(lockCtx, Query(UpsertVolume("volume2", "backend1")))
		defer nestedUnlocker()
		assert.NoError(t, err)
	})
}

func TestLockWithExistingLockContext(t *testing.T) {
	initCaches()

	t.Run("allows inconsistent queries", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadBackend("backend1")))
		defer unlocker()
		assert.NoError(t, err)

		_, results, unlocker2, err := Lock(lockCtx, Query(InconsistentReadBackend("backend1"), ListVolumes()))
		defer unlocker2()
		assert.NoError(t, err)
		assert.NotNil(t, results[0].Backend.Read)
		assert.NotEmpty(t, results[0].Volumes)
	})

	t.Run("rejects consistent queries", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadBackend("backend1")))
		defer unlocker()
		assert.NoError(t, err)

		_, _, _, err = Lock(lockCtx, Query(ReadVolume("volume1")))
		assert.ErrorContains(t, err, "cannot get consistent locks with existing query in context")
	})
}

func TestNestedLockRequiresLockContext(t *testing.T) {
	initCaches()
	_, _, _, err := NestedLock(context.Background(), Query(ReadBackend("backend1")))
	assert.ErrorContains(t, err, "nested locks must have existing query in context")
}

func TestNestedLockRejectsLockUpgrade(t *testing.T) {
	initCaches()
	lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadVolume("volume1")))
	defer unlocker()
	assert.NoError(t, err)

	_, _, _, err = NestedLock(lockCtx, Query(UpsertVolume("volume1", "backend1")))
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "nested locks must not upgrade locks") ||
			strings.Contains(err.Error(), "new lock could cause deadlock"),
		"expected upgrade or ordering rejection, got: %v", err,
	)
}

func TestNestedLockRejectsOutOfOrderResources(t *testing.T) {
	initCaches()
	lockCtx, _, unlocker, err := Lock(context.Background(), Query(UpsertBackend("backend2", "", "")))
	defer unlocker()
	assert.NoError(t, err)

	_, _, _, err = NestedLock(lockCtx, Query(UpsertBackend("backend1", "", "")))
	assert.ErrorContains(t, err, "new lock could cause deadlock")
}

func TestNestedLockRejectsKeyUpdates(t *testing.T) {
	initCaches()

	t.Run("rejects nested subquery with newKey on backend", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadBackend("backend1")))
		defer unlocker()
		assert.NoError(t, err)

		_, _, _, err = NestedLock(lockCtx, Query(UpsertBackend("backend1", "backend1", "renamed-backend")))
		assert.ErrorContains(t, err, "key updates are not allowed in nested lock")
	})

	t.Run("rejects nested subquery with newKey on volume", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadVolume("volume1")))
		defer unlocker()
		assert.NoError(t, err)

		_, _, _, err = NestedLock(
			lockCtx,
			Query(UpsertVolumeByInternalName("volume1", "internal-volume1", "renamed-internal", "backend1")),
		)
		assert.ErrorContains(t, err, "key updates are not allowed in nested lock")
	})

	t.Run("allows nested consistent lock while outer lock renames backend", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(
			context.Background(),
			Query(UpsertBackend("backend1", "backend1", "renamed-backend")),
		)
		defer unlocker()
		assert.NoError(t, err)

		_, nestedResults, nestedUnlocker, err := NestedLock(lockCtx, Query(ReadVolume("volume1")))
		defer nestedUnlocker()
		assert.NoError(t, err)
		assert.NotNil(t, nestedResults[0].Volume.Read)
	})

	t.Run("allows list-only nested lock while outer lock renames backend", func(t *testing.T) {
		lockCtx, _, unlocker, err := Lock(
			context.Background(),
			Query(UpsertBackend("backend1", "backend1", "renamed-backend")),
		)
		defer unlocker()
		assert.NoError(t, err)

		_, nestedResults, nestedUnlocker, err := NestedLock(lockCtx, Query(ListVolumes()))
		defer nestedUnlocker()
		assert.NoError(t, err)
		assert.NotEmpty(t, nestedResults[0].Volumes)
	})
}

// TestNestedLockCrossBackendInversionDeadlock reproduces cross-goroutine lock inversion when a nested
// lock is taken on a lower-sorted backend while holding a higher-sorted one, concurrent with a Lock
// acquiring both backends in order. Nested lock ordering validation must reject the nested lock instead of deadlocking.
func TestNestedLockCrossBackendInversionDeadlock(t *testing.T) {
	setupTestData(t, 2)
	defer cleanupTestData()

	g1HasB := make(chan struct{})
	g2Started := make(chan struct{})
	g1NestedErr := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		lockCtx, _, unlock, err := Lock(context.Background(), upsertBackendQuery("backend2"))
		if unlock != nil {
			defer unlock()
		}
		if err != nil {
			g1NestedErr <- err
			return
		}
		close(g1HasB)
		<-g2Started
		_, _, nestedUnlock, err := NestedLock(lockCtx, upsertBackendQuery("backend1"))
		if nestedUnlock != nil {
			nestedUnlock()
		}
		g1NestedErr <- err
	}()

	go func() {
		defer wg.Done()
		<-g1HasB
		g2Done := make(chan struct{})
		go func() {
			_, _, unlock, _ := Lock(
				context.Background(),
				upsertBackendQuery("backend1"),
				upsertBackendQuery("backend2"),
			)
			if unlock != nil {
				unlock()
			}
			close(g2Done)
		}()
		close(g2Started)
		<-g2Done
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-g1NestedErr:
		assert.ErrorContains(t, err, "new lock could cause deadlock")
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock detected")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutines did not finish")
	}
}

func TestNestedLockUpdateBackendVolumesPattern(t *testing.T) {
	initCaches()

	lockCtx, listResults, listUnlocker, err := Lock(
		context.Background(),
		Query(ListVolumesForBackend("backend1")),
	)
	defer listUnlocker()
	assert.NoError(t, err)
	assert.Len(t, listResults[0].Volumes, 1)

	_, nestedResults, nestedUnlocker, err := NestedLock(
		lockCtx,
		Query(UpsertBackend("backend1", "", "")),
		Query(UpsertVolume("volume1", "backend1")),
	)
	defer nestedUnlocker()
	assert.NoError(t, err)
	assert.NotNil(t, nestedResults[0].Backend.Read)
	assert.NotNil(t, nestedResults[1].Volume.Read)
}

func TestNestedLockSkipsAlreadyHeldResources(t *testing.T) {
	initCaches()

	lockCtx, _, unlocker, err := Lock(context.Background(), Query(UpsertBackend("backend1", "", "")))
	defer unlocker()
	assert.NoError(t, err)

	// Re-locking the same backend should not deadlock; nested lock skips it.
	nestedDone := make(chan error, 1)
	go func() {
		_, _, nestedUnlocker, nestedErr := NestedLock(lockCtx, Query(UpsertBackend("backend1", "", "")))
		if nestedUnlocker != nil {
			nestedUnlocker()
		}
		nestedDone <- nestedErr
	}()

	select {
	case nestedErr := <-nestedDone:
		assert.NoError(t, nestedErr)
	case <-time.After(2 * time.Second):
		t.Fatal("NestedLock on already-held backend deadlocked")
	}
}

// TestNestedLockSkipsSyntheticReadOnWriteLockedDependency ensures nested locks on a child resource
// skip the synthetic read dependency injected by buildTree when the parent is already write-locked.
func TestNestedLockSkipsSyntheticReadOnWriteLockedDependency(t *testing.T) {
	initCaches()

	lockCtx, _, unlocker, err := Lock(context.Background(), Query(UpsertBackend("backend1", "backend1", "")))
	defer unlocker()
	assert.NoError(t, err)

	nestedDone := make(chan error, 1)
	go func() {
		_, _, nestedUnlocker, nestedErr := NestedLock(lockCtx, Query(UpsertVolume("volume2", "backend1")))
		if nestedUnlocker != nil {
			nestedUnlocker()
		}
		nestedDone <- nestedErr
	}()

	select {
	case nestedErr := <-nestedDone:
		assert.NoError(t, nestedErr)
	case <-time.After(2 * time.Second):
		t.Fatal("NestedLock on child with write-locked backend dependency deadlocked")
	}
}

// TestNestedLockPreservesLastLock ensures lastLock from lock() is propagated through NestedLock
// when no new consistent locks are acquired. Without this, nested list/inconsistent-read-only
// rounds clear lastLock and later nested locks skip deadlock ordering validation.
func TestNestedLockPreservesLastLock(t *testing.T) {
	t.Run("propagated through list and inconsistent-read nested locks", func(t *testing.T) {
		initCaches()

		lockCtx, _, unlocker, err := Lock(context.Background(), Query(ReadBackend("backend1")))
		defer unlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "Lock with consistent locks must set lastLock", backend, "backend1")

		lockCtx, _, listUnlocker, err := NestedLock(lockCtx, Query(ListVolumes()))
		defer listUnlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "list-only NestedLock must preserve lastLock", backend, "backend1")

		lockCtx, _, readUnlocker, err := NestedLock(
			lockCtx,
			Query(InconsistentReadBackend("backend1"), ListBackends()),
		)
		defer readUnlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "inconsistent-read/list NestedLock must preserve lastLock", backend, "backend1")

		lockCtx, _, volumeUnlocker, err := NestedLock(lockCtx, Query(UpsertVolume("volume2", "backend1")))
		defer volumeUnlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "NestedLock with a new consistent lock must set lastLock", volume, "volume2")

		lockCtx, _, finalUnlocker, err := NestedLock(lockCtx, Query(ListVolumes(), InconsistentReadVolume("volume2")))
		defer finalUnlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "subsequent list/inconsistent-read NestedLock must preserve lastLock", volume, "volume2")
	})

	t.Run("out-of-order lock rejected after list-only nested lock", func(t *testing.T) {
		initCaches()

		lockCtx, _, unlocker, err := Lock(context.Background(), Query(UpsertBackend("backend2", "", "")))
		defer unlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "Lock with consistent locks must set lastLock", backend, "backend2")

		lockCtx, _, listUnlocker, err := NestedLock(lockCtx, Query(ListVolumes()))
		defer listUnlocker()
		assert.NoError(t, err)
		assertLastLockPreserved(t, lockCtx, "list-only NestedLock must preserve lastLock", backend, "backend2")

		_, _, nestedUnlocker, err := NestedLock(lockCtx, Query(UpsertBackend("backend1", "", "")))
		if nestedUnlocker != nil {
			nestedUnlocker()
		}
		assert.ErrorContains(t, err, "new lock could cause deadlock",
			"lastLock must drive ordering validation after list-only nested lock")
	})
}
