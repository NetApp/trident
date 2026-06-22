// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package concurrent_cache provides ordered, deadlock-safe locking for Trident's in-memory
// resource caches.
//
// Subqueries in a single Query() slice are deduplicated to at most one non-list operation per
// resource type (list plus one other op on the same type is allowed). Duplicate non-list ops
// on the same type return an error; for example, two UpsertBackend calls in one Query().
//
// Locks are acquired in a stable order: list ops first, then resource rank, resource type,
// id, and operation. New keys are locked by lockNewKeys before resource locks in lockQuery.
//
// NestedLock rejects lock upgrades, out-of-order new locks relative to the last held lock,
// and nested subqueries with newKey.

package concurrent_cache

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func init() {
	Initialize()
}

// Initialize resets global cache state. Exported for unit tests.
func Initialize() {
	rootCache = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	nodes = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	storageClasses = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	groupSnapshots = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	backends = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
		// backend name has to be unique
		key: &uniqueKey{
			data:     make(map[string]string),
			keyLocks: locks.NewGCNamedMutex(),
		},
		allowBlankID: true,
	}
	volumes = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
		// volume internal name
		key: &uniqueKey{
			data:     make(map[string]string),
			keyLocks: locks.NewGCNamedMutex(),
		},
	}
	subordinateVolumes = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	volumePublications = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	snapshots = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	autogrowPolicies = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}

	inverseSchema = make(map[resource][]resource, len(schema))
	for r, c := range schema {
		for _, o := range c {
			inverseSchema[o] = append(inverseSchema[o], r)
		}
	}

	resourceRanks = make(map[resource]int, len(schema))
	for r := range schema {
		resourceRanks[r] = rankForResource(r)
	}

	rootCache.data["."] = SmartCopier(nil)
}

// SmartCopier is an interface for objects that can be copied for the cache. If the object implements interior
// mutability, it may be implemented as a shallow copy. If not, it is implemented as a deep copy.
type SmartCopier interface {
	SmartCopy() interface{}
}

type cache struct {
	// map of resource id to resource
	data map[string]SmartCopier
	// locks for each resource, by id
	resourceLocks *locks.GCNamedMutex
	// lock for adding or removing data entries
	m sync.RWMutex
	// optional unique key
	key *uniqueKey
	// allowBlankID allows a resource to manage its own ID on create, such as generating a UUID
	allowBlankID bool
}

type uniqueKey struct {
	// map of view to resource id
	data map[string]string
	// locks for each view, by key string (only required if view must be unique)
	keyLocks *locks.GCNamedMutex
	// lock for adding or removing data entries
	m sync.RWMutex
}

func (c *cache) lock() {
	if c.key != nil {
		c.key.m.Lock()
	}
	c.m.Lock()
}

func (c *cache) unlock() {
	c.m.Unlock()
	if c.key != nil {
		c.key.m.Unlock()
	}
}

func (c *cache) rlock() {
	if c.key != nil {
		c.key.m.RLock()
	}
	c.m.RLock()
}

func (c *cache) runlock() {
	c.m.RUnlock()
	if c.key != nil {
		c.key.m.RUnlock()
	}
}

var (
	rootCache          cache
	nodes              cache
	storageClasses     cache
	groupSnapshots     cache
	backends           cache
	volumes            cache
	subordinateVolumes cache
	volumePublications cache
	snapshots          cache
	autogrowPolicies   cache

	caches = map[resource]*cache{
		root:              &rootCache,
		node:              &nodes,
		storageClass:      &storageClasses,
		groupSnapshot:     &groupSnapshots,
		backend:           &backends,
		volume:            &volumes,
		subordinateVolume: &subordinateVolumes,
		volumePublication: &volumePublications,
		snapshot:          &snapshots,
		autogrowPolicy:    &autogrowPolicies,
	}

	inverseSchema map[resource][]resource
	resourceRanks map[resource]int
)

type Subquery struct {
	// resource type
	res resource
	// operation type
	op operation
	// dependency indices in Subquery slice
	dependencies []int
	// id of the resource
	id string
	// key of the resource if id is not provided
	key string
	// newKey is used to update the key of the resource
	newKey string
	// setDependencyIDs sets dependency IDs from dependency indices in the Subquery slice
	setDependencyIDs func(s []Subquery, i int) error
	// setResults is a function that sets Result for this subquery
	setResults func(*Subquery, *Result) error
	// result is the index of the Result slice that this subquery belongs to
	result int
	// err is an error that occurred while creating this subquery
	err error
}

func (sq Subquery) elem() querySetElem {
	return querySetElem{
		id:  sq.id,
		res: sq.res,
	}
}

func Query(query ...Subquery) []Subquery {
	return query
}

type querySetElem struct {
	res resource
	id  string
}

func (e querySetElem) subquery() Subquery {
	return Subquery{
		res: e.res,
		id:  e.id,
	}
}

type querySet map[querySetElem]bool
type lockContextKey struct{}
type lockContext struct {
	qs               querySet
	lastLock         *Subquery
	hasSyntheticRoot bool
}

// Lock locks resources from queries. Returns a context that can be used with NestedLock.
func Lock(ctx context.Context, queries ...[]Subquery) (context.Context, []Result, func(), error) {
	roots, cachesPresent, hasConsistentLocks, _, err := assembleQueries(queries)
	if err != nil {
		return ctx, nil, func() {}, err
	}

	if hasConsistentLocks && ctx.Value(lockContextKey{}) != nil {
		return ctx, nil, func() {}, fmt.Errorf("cannot get consistent locks with existing query in context")
	}

	ret, err := lock(nil, nil, false, roots, cachesPresent, hasConsistentLocks, queries...)
	if err != nil {
		return ctx, nil, ret.unlocker, err
	}
	if ctx.Err() != nil {
		return ctx, nil, ret.unlocker, ctx.Err()
	}
	lockCtx := context.WithValue(ctx, lockContextKey{}, &lockContext{
		qs:               ret.qs,
		lastLock:         ret.lastLock,
		hasSyntheticRoot: ret.hasConsistentLocks,
	})
	return lockCtx, ret.results, ret.unlocker, nil
}

// NestedLock extends locks held in ctx. New locks must not upgrade existing read locks, must sort
// after the last held lock, and must not include newKey. Returns an error otherwise.
func NestedLock(ctx context.Context, queries ...[]Subquery) (context.Context, []Result, func(), error) {
	lockContextValue := ctx.Value(lockContextKey{})
	if lockContextValue == nil {
		return ctx, nil, func() {}, fmt.Errorf("nested locks must have existing query in context")
	}

	ql := lockContextValue.(*lockContext)
	roots, cachesPresent, hasConsistentLocks, hasNewKeys, err := assembleQueries(queries)
	if err != nil {
		return ctx, nil, func() {}, err
	}
	if hasNewKeys {
		return ctx, nil, func() {}, fmt.Errorf("key updates are not allowed in nested lock")
	}

	ret, err := lock(ql.qs, ql.lastLock, ql.hasSyntheticRoot, roots, cachesPresent, hasConsistentLocks, queries...)
	if err != nil {
		return ctx, nil, ret.unlocker, err
	}
	if ctx.Err() != nil {
		return ctx, nil, ret.unlocker, ctx.Err()
	}
	lockCtx := context.WithValue(ctx, lockContextKey{}, &lockContext{
		qs:               ret.qs,
		lastLock:         ret.lastLock,
		hasSyntheticRoot: ret.hasConsistentLocks,
	})
	return lockCtx, ret.results, ret.unlocker, nil
}

type lockReturn struct {
	results            []Result
	qs                 querySet
	lastLock           *Subquery
	hasConsistentLocks bool
	hasKeyUpdate       bool
	unlocker           func()
}

// lock acquires locks for assembled queries. Callers must run assembleQueries first and reject
// nested newKey updates before calling lock.
//  1. Fill in IDs under cache read locks.
//  2. Merge queries and validate nested locks (no read-to-write upgrades; first new consistent
//     lock must sort after lastLock).
//  3. Lock new keys (lockNewKeys), then acquire resource locks and populate results (lockQuery).
//  4. Build return context: merged query set, lastLock, and results.
func lock(existingQuerySet querySet, lastLock *Subquery, hasSyntheticRoot bool,
	roots [][]int, cachesPresent map[resource]struct{}, hasConsistentLocks bool,
	queries ...[]Subquery) (ret lockReturn, err error) {
	ret.unlocker = func() {}
	// phase 1, requires locks, fill in IDs
	if err = lockCachesAndFillInIDs(queries, roots, cachesPresent); err != nil {
		return ret, err
	}

	// phase 2, merge and validate with existing locks
	newKeys, merged := mergeQueries(queries, hasConsistentLocks && !hasSyntheticRoot)
	// no new locks can upgrade
	for _, q := range merged {
		if write, ok := existingQuerySet[q.elem()]; ok && !write && isWriteOp(q.op) {
			return ret, fmt.Errorf("nested locks must not upgrade locks: [%s: %s]", q.res, q.op)
		}
	}
	// the first new consistent lock must be after the last lock held to avoid deadlocks
	if lastLock != nil {
		for _, q := range merged {
			if q.op == list || q.op == inconsistentRead {
				continue
			}
			if _, ok := existingQuerySet[q.elem()]; ok {
				continue
			}
			if compareSubqueries(*lastLock, q) > 0 {
				return ret, fmt.Errorf("new lock could cause deadlock: [%s: %s]", q.res, q.op)
			}
			break
		}
	}

	// phase 4, acquire locks, fill in results
	newKeyUnlocker, err := lockNewKeys(newKeys)
	if err != nil {
		ret.unlocker = newKeyUnlocker
		return ret, err
	}
	results, unlocker, err := lockQuery(existingQuerySet, merged, len(queries))
	ret.unlocker = func() {
		unlocker()
		newKeyUnlocker()
	}
	if err != nil {
		return ret, err
	}

	// phase 5, return elements of new context
	qs := make(querySet)
	for k, v := range existingQuerySet {
		qs[k] = v
	}
	lastLockIndex := -1
	for i, q := range merged {
		if q.op == list || q.op == inconsistentRead {
			continue
		}
		lastLockIndex = i
		qs[q.elem()] = isWriteOp(q.op)
	}
	if lastLockIndex > -1 {
		sq := merged[lastLockIndex]
		lastLock = &sq
	}
	ret.results = results
	ret.qs = qs
	ret.lastLock = lastLock
	ret.hasConsistentLocks = hasConsistentLocks || hasSyntheticRoot
	for _, q := range merged {
		if q.newKey == "" {
			continue
		}
		if _, ok := existingQuerySet[q.elem()]; ok {
			continue
		}
		ret.hasKeyUpdate = true
		break
	}
	return ret, nil
}

func assembleQueries(queries [][]Subquery) (roots [][]int, cachesPresent map[resource]struct{}, hasConsistentLocks,
	hasNewKeys bool, err error) {
	cachesPresent = make(map[resource]struct{}, resourceCount)
	roots = make([][]int, len(queries))
	for i, q := range queries {
		if err := checkError(q); err != nil {
			return nil, nil, false, false, err
		}
		ri, err := dedupe(q)
		if err != nil {
			return nil, nil, false, false, err
		}
		queries[i], roots[i] = buildTrees(ri, q)
		for _, sq := range queries[i] {
			if sq.newKey != "" {
				hasNewKeys = true
			}
			cachesPresent[sq.res] = struct{}{}
			if sq.op != list && sq.op != inconsistentRead {
				hasConsistentLocks = true
			}
		}
	}

	return roots, cachesPresent, hasConsistentLocks, hasNewKeys, nil
}

func lockCachesAndFillInIDs(queries [][]Subquery, roots [][]int, cachesPresent map[resource]struct{}) error {
	resources := make([]resource, 0, len(cachesPresent))
	for r := range cachesPresent {
		resources = append(resources, r)
	}
	unlocker, err := rLockCaches(resources)
	defer unlocker()
	if err != nil {
		return err
	}

	for i := range queries {
		for _, r := range roots[i] {
			if err := fillInIDs(r, queries[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// rLockCaches sorts resources by descending rank then resource type, acquires read locks on
// each cache, and returns an unlock function. Returns an error if a cache does not exist.
func rLockCaches(resources []resource) (func(), error) {
	slices.SortFunc(resources, func(i, j resource) int {
		switch {
		case resourceRanks[i] > resourceRanks[j]:
			return -1
		case resourceRanks[i] < resourceRanks[j]:
			return 1
		}

		switch {
		case i < j:
			return -1
		case i > j:
			return 1
		}

		return 0
	})
	unlocks := make([]func(), 0, len(caches))
	for _, r := range resources {
		c, ok := caches[r]
		if !ok {
			return func() {}, fmt.Errorf("no cache found for resource %s", r)
		}
		c.rlock()
		unlocks = append(unlocks, c.runlock)
	}
	return func() {
		doReverse(unlocks)
	}, nil
}

func checkError(queries []Subquery) error {
	for _, q := range queries {
		if q.err != nil {
			return q.err
		}
	}
	return nil
}

// dedupe takes a slice of subqueries and returns a map of resource to index in the slice, and an error if there are
// any duplicates. As a side effect it sorts the slice.
func dedupe(query []Subquery) (map[resource]int, error) {
	resourceIndices := make(map[resource]int, len(query))
	if len(query) == 0 {
		return resourceIndices, nil
	}

	slices.SortFunc(query, func(i, j Subquery) int {
		switch {
		case i.op == list && j.op != list:
			return -1
		case i.op != list && j.op == list:
			return 1
		}

		switch {
		case i.res < j.res:
			return -1
		case i.res > j.res:
			return 1
		}

		switch {
		case i.op < j.op:
			return -1
		case i.op > j.op:
			return 1
		}

		return 0
	})

	subquery := query[0]
	if subquery.op != list {
		resourceIndices[subquery.res] = 0
	}
	for i := 1; i < len(query); i++ {
		// If there is only one resource in the query, a list and RUD operation will appear duplicate without the second
		// clause
		if query[i].res == subquery.res && !(subquery.op == list && query[i].op != list) {
			return nil, fmt.Errorf("duplicate %s operation (%s and %s)", subquery.res, query[i].op, subquery.op)
		}
		subquery = query[i]
		if subquery.op != list {
			resourceIndices[subquery.res] = i
		}
	}

	return resourceIndices, nil
}

// buildTrees takes subqueries and finds the roots, and fills in any missing dependencies
func buildTrees(resourceIndices map[resource]int, query []Subquery) ([]Subquery, []int) {
	roots := make([]int, 0)
	l := len(query)
	for i := 0; i < l; i++ {
		query = buildTree(resourceIndices, query, i)
	}
	for i := 0; i < l; i++ {
		// lists and inconsistent reads are always roots
		if query[i].op == list || query[i].op == inconsistentRead {
			roots = append(roots, i)
			continue
		}

		// if a dependent resource type is also present in the query, this subquery is not a root
		root := true
		dependents := inverseSchema[query[i].res]
		for j := 0; j < len(dependents) && root; j++ {
			if _, ok := resourceIndices[dependents[j]]; ok {
				root = false
			}
		}
		if root {
			roots = append(roots, i)
		}
	}
	return query, roots
}

// buildTree walks schema dependencies for a subquery, appending synthetic read subqueries as needed.
func buildTree(resourceIndices map[resource]int, query []Subquery, root int) []Subquery {
	if query[root].op == list || query[root].op == inconsistentRead {
		return query
	}
	// find or create all the dependencies
	for _, owner := range schema[query[root].res] {
		i, ok := resourceIndices[owner]
		if !ok {
			query = append(query, Subquery{res: owner, op: read})
			i = len(query) - 1
			resourceIndices[owner] = i
		}
		if !slices.Contains(query[root].dependencies, i) {
			query[root].dependencies = append(query[root].dependencies, i)
		}
		query = buildTree(resourceIndices, query, i)
	}
	return query
}

// fillInIDs finds all missing IDs in a tree. An ID may be:
// 1. Passed in the subquery
// 2. Resolved from key via uniqueKey.data
// 3. Propagated to dependency subqueries from stored dependent data (dependentQueries)
// 4. Set explicitly via setDependencyIDs
func fillInIDs(root int, query []Subquery) error {
	c := caches[query[root].res]
	if query[root].op == list {
		return nil
	}
	if query[root].id == "" && query[root].setDependencyIDs == nil && query[root].key == "" {
		return nil
	}
	if c.key != nil {
		if query[root].newKey != "" {
			if _, ok := c.key.data[query[root].newKey]; ok {
				return fmt.Errorf("key %s for %s already exists", query[root].newKey, query[root].res)
			}
		}
		if query[root].key != "" {
			id, ok := c.key.data[query[root].key]
			if !ok && !c.allowBlankID {
				return errors.NotFoundError("no %s found with key %s", query[root].res, query[root].key)
			}
			if id != "" && query[root].id != "" && query[root].id != id {
				return fmt.Errorf("conflicting ids for %s: [%s] and [%s]", query[root].res, id, query[root].id)
			}
			query[root].id = id
		} else {
			res, ok := c.data[query[root].id]
			if ok {
				query[root].key = res.(UniqueKey).GetUniqueKey()
			}
		}
	}
	if query[root].setDependencyIDs != nil {
		if err := query[root].setDependencyIDs(query, root); err != nil {
			return err
		}
	}
	for _, owner := range query[root].dependencies {
		if query[root].setDependencyIDs == nil {
			res, ok := c.data[query[root].id]
			if !ok {
				return nil
			}
			getter, ok := dependentQueries[query[owner].res]
			if !ok {
				return fmt.Errorf("no dependent query found for %s", query[owner].res)
			}
			id, err := getter(res)
			if err != nil {
				return err
			}
			if id != "" && query[owner].id != "" && id != query[owner].id {
				return fmt.Errorf("conflicting ids for %s: (%s) and (%s)", query[owner].res, id, query[owner].id)
			}
			query[owner].id = id
		}
		if err := fillInIDs(owner, query); err != nil {
			return err
		}
	}
	return nil
}

// subqueries are sorted by:
// 1. list operations
// 2. resource rank
// 3. resource type
// 4. resource id
// 5. operation
func compareSubqueries(i, j Subquery) int {
	switch {
	case i.op == list && j.op != list:
		return -1
	case i.op != list && j.op == list:
		return 1
	}

	switch {
	case resourceRanks[i.res] < resourceRanks[j.res]:
		return -1
	case resourceRanks[i.res] > resourceRanks[j.res]:
		return 1
	}

	switch {
	case i.res < j.res:
		return -1
	case i.res > j.res:
		return 1
	}

	switch {
	case i.id < j.id:
		return -1
	case i.id > j.id:
		return 1
	}

	switch {
	case i.op < j.op:
		return -1
	case i.op > j.op:
		return 1
	}

	return 0
}

type newKey struct {
	r resource
	k string
	o operation
}

func compareNewKeys(i, j newKey) int {
	switch {
	case i.r < j.r:
		return -1
	case i.r > j.r:
		return 1
	}
	switch {
	case i.k < j.k:
		return -1
	case i.k > j.k:
		return 1
	}
	switch {
	case i.o < j.o:
		return -1
	case i.o > j.o:
		return 1
	}
	return 0
}

// lockNewKeys locks all new keys in query. Returns error if they collide with existing resources.
func lockNewKeys(newKeys []newKey) (func(), error) {
	unlocks := make([]func(), 0, len(newKeys))
	var previousNK *newKey
	for _, nk := range newKeys {
		c := caches[nk.r]
		if c.key == nil {
			continue
		}
		unlock := lockNewKey(previousNK, nk, c)
		if unlock != nil {
			unlocks = append(unlocks, unlock)
		}
		previousNK = &nk
		// check for collisions after locking, to ensure the search is
		// consistent. It is the caller's responsibility to handle the error and
		// call unlock. Key updates are rare, so it's ok to take each cache lock individually instead of combining them
		// like we do for fillInIDs.
		c.key.m.RLock()
		_, exists := c.key.data[nk.k]
		c.key.m.RUnlock()
		if exists {
			return func() { doReverse(unlocks) }, fmt.Errorf("key %s for %s already exists", nk.k, nk.r)
		}
	}
	return func() { doReverse(unlocks) }, nil
}

func lockNewKey(previousNK *newKey, nk newKey, c *cache) func() {
	if previousNK == nil || previousNK.r != nk.r || previousNK.k != nk.k {
		if isWriteOp(nk.o) {
			c.key.keyLocks.Lock(nk.k)
			return func() {
				c.key.keyLocks.Unlock(nk.k)
			}
		} else {
			c.key.keyLocks.RLock(nk.k)
			return func() {
				c.key.keyLocks.RUnlock(nk.k)
			}
		}
	}
	return nil
}

func mergeQueries(queries [][]Subquery, hasConsistentLocks bool) ([]newKey, []Subquery) {
	length := 1
	for _, q := range queries {
		length += len(q)
	}
	merged := make([]Subquery, 0, length)
	newKeys := make([]newKey, 0, length)
	// if there are consistent locks, we need to add the root lock to block during shutdown. Inconsistent reads are still allowed.
	if hasConsistentLocks {
		merged = append(merged, Subquery{res: root, op: read, id: "."})
	}
	for i, q := range queries {
		for j := range q {
			q[j].result = i
			if q[j].newKey != "" {
				newKeys = append(newKeys, newKey{q[j].res, q[j].newKey, q[j].op})
			}
		}
		merged = append(merged, q...)
	}
	slices.SortFunc(merged, compareSubqueries)
	slices.SortFunc(newKeys, compareNewKeys)
	return newKeys, merged
}

// lockQuery acquires resource and existing-key locks for new subqueries in stable order
// (from mergeQueries), then populates results. New keys are locked by lockNewKeys before
// this function runs.
func lockQuery(existingQuerySet querySet, query []Subquery, resultCount int) ([]Result, func(), error) {
	results := make([]Result, resultCount)
	unlocks := make([]func(), 0, len(query))

	// Lock keys with their resources in subquery order.
	var lastLocked *Subquery
	for i := range query {
		if _, ok := existingQuerySet[query[i].elem()]; ok {
			continue
		}
		if query[i].op == list || query[i].op == inconsistentRead {
			continue
		}
		allowBlankID := caches[query[i].res].allowBlankID
		if query[i].id == "" && !allowBlankID {
			continue
		}
		u := lockSubquery(&query[i], lastLocked, allowBlankID)
		unlocks = append(unlocks, u...)
		lastLocked = &query[i]
	}

	// set results
	for i := range query {
		if query[i].setResults != nil && (query[i].id != "" || caches[query[i].res].allowBlankID || query[i].op == list) {
			if err := query[i].setResults(&query[i], &results[query[i].result]); err != nil {
				return nil, func() { doReverse(unlocks) }, err
			}
		}
	}

	return results, func() { doReverse(unlocks) }, nil
}

func lockSubquery(subquery, previousSubquery *Subquery, allowBlankID bool) []func() {
	c := caches[subquery.res]
	write := isWriteOp(subquery.op)
	unlocks := make([]func(), 0, 2)

	if (subquery.id != "" || !allowBlankID) &&
		(previousSubquery == nil || previousSubquery.res != subquery.res || previousSubquery.id != subquery.id) {
		if subquery.key != "" {
			if write {
				c.key.keyLocks.Lock(subquery.key)
				unlocks = append(unlocks, func() {
					c.key.keyLocks.Unlock(subquery.key)
				})
			} else {
				c.key.keyLocks.RLock(subquery.key)
				unlocks = append(unlocks, func() {
					c.key.keyLocks.RUnlock(subquery.key)
				})
			}
		}
		if write {
			c.resourceLocks.Lock(subquery.id)
			unlocks = append(unlocks, func() {
				c.resourceLocks.Unlock(subquery.id)
			})
		} else {
			c.resourceLocks.RLock(subquery.id)
			unlocks = append(unlocks, func() {
				c.resourceLocks.RUnlock(subquery.id)
			})
		}
	}

	return unlocks
}

func doReverse(f []func()) {
	for i := len(f) - 1; i >= 0; i-- {
		f[i]()
	}
}

func isWriteOp(op operation) bool {
	switch op {
	case list, read, inconsistentRead:
		return false
	default:
		return true
	}
}

func checkDependency(s []Subquery, i int, r resource) error {
	if len(s[i].dependencies) != 1 {
		return fmt.Errorf("expected one dependency, got %d", len(s[i].dependencies))
	}
	if s[s[i].dependencies[0]].res != r {
		return fmt.Errorf("expected %s dependency, got %s", r, s[s[i].dependencies[0]].res)
	}
	return nil
}

func rankForResource(r resource) int {
	rank := 0
	for _, c := range schema[r] {
		cRank := 1 + rankForResource(c)
		if cRank > rank {
			rank = cRank
		}
	}
	return rank
}

type Result struct {
	Nodes              []*models.Node
	StorageClasses     []*storageclass.StorageClass
	Backends           []storage.Backend
	Volumes            []*storage.Volume
	SubordinateVolumes []*storage.Volume
	VolumePublications []*models.VolumePublication
	Snapshots          []*storage.Snapshot
	AutogrowPolicies   []*storage.AutogrowPolicy
	GroupSnapshots     []*storage.GroupSnapshot

	Node              NodeResult
	StorageClass      StorageClassResult
	Backend           BackendResult
	Volume            VolumeResult
	SubordinateVolume SubordinateVolumeResult
	VolumePublication VolumePublicationResult
	Snapshot          SnapshotResult
	AutogrowPolicy    AutogrowPolicyResult
	GroupSnapshot     GroupSnapshotResult
}

type NodeResult struct {
	Read   *models.Node
	Upsert func(*models.Node)
	Delete func()
}

type StorageClassResult struct {
	Read   *storageclass.StorageClass
	Upsert func(*storageclass.StorageClass)
	Delete func()
}

type BackendResult struct {
	Read   storage.Backend
	Upsert func(storage.Backend)
	Delete func()
}

type VolumeResult struct {
	Read   *storage.Volume
	Upsert func(*storage.Volume)
	Delete func()
}

type SubordinateVolumeResult struct {
	Read   *storage.Volume
	Upsert func(*storage.Volume)
	Delete func()
}

type VolumePublicationResult struct {
	Read   *models.VolumePublication
	Upsert func(*models.VolumePublication)
	Delete func()
}

type SnapshotResult struct {
	Read   *storage.Snapshot
	Upsert func(*storage.Snapshot)
	Delete func()
}

type AutogrowPolicyResult struct {
	Read   *storage.AutogrowPolicy
	Upsert func(*storage.AutogrowPolicy)
	Delete func()
}

type GroupSnapshotResult struct {
	Read   *storage.GroupSnapshot
	Upsert func(*storage.GroupSnapshot)
	Delete func()
}

type resource int

func (r resource) String() string {
	if n, ok := resourceNames[r]; ok {
		return n
	}
	return "Unknown"
}

var resourceNames = map[resource]string{
	node:              "Node",
	storageClass:      "Storage Class",
	groupSnapshot:     "Group Snapshot",
	backend:           "Backend",
	volume:            "Volume",
	subordinateVolume: "Subordinate Volume",
	volumePublication: "Volume Publication",
	snapshot:          "Snapshot",
	autogrowPolicy:    "Autogrow Policy",
}

const (
	root = resource(iota)
	node
	storageClass
	// groupSnapshot sorts before backend so a held group lock can legally NestedLock its constituent
	// backend->volume->snapshot trees. Always take the group lock first, never after a backend/volume/snapshot lock.
	groupSnapshot
	backend
	volume
	subordinateVolume
	volumePublication
	snapshot
	autogrowPolicy
	resourceCount = int(autogrowPolicy) + 1
)

type operation int

func (o operation) String() string {
	if n, ok := operationNames[o]; ok {
		return n
	}
	return "Unknown"
}

var operationNames = map[operation]string{
	list:             "List",
	upsert:           "Upsert",
	del:              "Delete",
	read:             "Read",
	inconsistentRead: "Inconsistent Read",
}

const (
	list = operation(iota)
	upsert
	del
	read
	inconsistentRead
)

// schema is the authoritative source for relationships between resources. For example,
// a snapshot depends on a volume, and a volume depends on a backend.
var schema = map[resource][]resource{
	root:              nil, // root is not a schema dependency; injected synthetically during consistent locks
	node:              nil,
	storageClass:      nil,
	groupSnapshot:     nil, // parentless; constituents are locked as separate snapshot trees
	backend:           nil,
	autogrowPolicy:    nil,
	volume:            {backend},
	subordinateVolume: {volume},
	volumePublication: {volume, node},
	snapshot:          {volume},
}

var dependentQueries = map[resource]func(interface{}) (string, error){
	backend: func(i interface{}) (string, error) {
		c, ok := i.(BackendDependent)
		if !ok {
			return "", fmt.Errorf("not owned by backend")
		}
		return c.GetBackendID(), nil
	},
	volume: func(i interface{}) (string, error) {
		c, ok := i.(VolumeDependent)
		if !ok {
			return "", fmt.Errorf("not a child of volume")
		}
		return c.GetVolumeID(), nil
	},
	node: func(i interface{}) (string, error) {
		c, ok := i.(NodeDependent)
		if !ok {
			return "", fmt.Errorf("not a child of node")
		}
		return c.GetNodeID(), nil
	},
}

type BackendDependent interface {
	GetBackendID() string
}

type VolumeDependent interface {
	GetVolumeID() string
}

type NodeDependent interface {
	GetNodeID() string
}

type UniqueKey interface {
	GetUniqueKey() string
}

type newKeyToLock struct {
	r resource
	k string
}
