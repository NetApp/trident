// Package concurrent_cache provides similar functionality as concurrent_cache but with half the calories
// Resources/objects/whatever are always locked and unlocked in the same order, by list, then type, then id.
// One query may only contain list operations and one "tree" of resources.
// Multiple "trees" must be found with multiple queries. For example, if volume 1 is in backend 1,
// and backend 2 and volume 1 are in the same query, that will return an error.

package concurrent_cache

import (
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

// Initialize Need this as public for unit tests
func Initialize() {
	nodes = cache{
		data:          make(map[string]SmartCopier),
		resourceLocks: locks.NewGCNamedMutex(),
	}
	storageClasses = cache{
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
	// locks for each view, by id (only required if view must be unique)
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
	nodes              cache
	storageClasses     cache
	backends           cache
	volumes            cache
	subordinateVolumes cache
	volumePublications cache
	snapshots          cache

	caches = map[resource]*cache{
		node:              &nodes,
		storageClass:      &storageClasses,
		backend:           &backends,
		volume:            &volumes,
		subordinateVolume: &subordinateVolumes,
		volumePublication: &volumePublications,
		snapshot:          &snapshots,
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
	// setDependencyIDs is a function that sets the owner IDs (based on owner indices) for the Subquery slice
	setDependencyIDs func(s []Subquery, i int) error
	// setResults is a function that sets Result for this subquery
	setResults func(*Subquery, *Result) error
	// result is the index of the Result slice that this subquery belongs to
	result int
	// err is an error that occurred while creating this subquery
	err error
}

func Query(query ...Subquery) []Subquery {
	return query
}

// Lock
//  1. Check for errors
//     Return errors from subquery generators
//  2. Dedupe
//     One list or crud operation for each resource is allowed (is called dedupe but returns an error)
//  3. Find parents/build trees
//     Returns roots of trees
//  4. Fill in IDs
//     This is where we get read locks on caches
//  5. Merge all queries
//     Returns sorted list of subqueries
//  6. Acquire locks and fill in Results
func Lock(queries ...[]Subquery) ([]Result, func(), error) {
	roots := make([][]int, len(queries))
	cachesPresent := make(map[resource]struct{}, resourceCount)
	// phase 1, requires no locks, check errors, dedupe, and build trees
	if err := assembleQueries(queries, roots, cachesPresent); err != nil {
		return nil, func() {}, err
	}

	// phase 2, requires locks, fill in IDs
	if err := lockCachesAndFillInIDs(queries, roots, cachesPresent); err != nil {
		return nil, func() {}, err
	}

	// phase 3, takes per-resource locks
	merged := mergeQueries(queries)
	return lockQuery(merged, len(queries))
}

func assembleQueries(queries [][]Subquery, roots [][]int, cachesPresent map[resource]struct{}) error {
	for i, q := range queries {
		if err := checkError(q); err != nil {
			return err
		}
		ri, err := dedupe(q)
		if err != nil {
			return err
		}
		for _, sq := range q {
			cachesPresent[sq.res] = struct{}{}
		}
		queries[i], roots[i] = buildTrees(ri, q)
	}
	return nil
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

// rLockCaches sorts and acquires read locks on all caches for the given resources. It returns a function that releases
// the locks, and error if a cache does not exist.
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
		if query[i].res == subquery.res {
			return nil, fmt.Errorf("duplicate %s operation", subquery.res)
		}
		subquery = query[i]
		if subquery.op != list {
			resourceIndices[subquery.res] = i
		}
	}

	return resourceIndices, nil
}

// buildTrees takes subqueries and finds the roots, and fills in any missing parents
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

		// if the current subquery has ownable resources in the query it is not a root
		root := true
		ownables := inverseSchema[query[i].res]
		for j := 0; j < len(ownables) && root; j++ {
			if _, ok := resourceIndices[ownables[j]]; ok {
				root = false
			}
		}
		if root {
			roots = append(roots, i)
		}
	}
	return query, roots
}

// buildTree takes a Subquery and builds a tree from it, adding existing subqueries as appropriate
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

// fillInIDs finds all missing IDs in a tree. There are 3 ways to set an ID:
// 1. It was passed in the subquery
// 2. It is set by a child
// 3. A key was passed in the subquery
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
			if query[root].id != "" && query[root].id != id {
				return fmt.Errorf("conflicting ids for %s: %s and %s", query[root].res, id, query[root].id)
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
			if query[owner].id != "" && id != query[owner].id {
				return fmt.Errorf("conflicting ids for %s: %s and %s", query[owner].res, id, query[owner].id)
			}
			query[owner].id = id
		}
		if err := fillInIDs(owner, query); err != nil {
			return err
		}
	}
	return nil
}

func mergeQueries(queries [][]Subquery) []Subquery {
	length := 0
	for _, q := range queries {
		length += len(q)
	}
	merged := make([]Subquery, 0, length)
	for i, q := range queries {
		for j := range q {
			q[j].result = i
		}
		merged = append(merged, q...)
	}
	// subqueries are sorted by:
	// 1. list operations
	// 2. resource rank
	// 3. resource type
	// 4. resource id
	// 5. operation
	slices.SortFunc(merged, func(i, j Subquery) int {
		switch {
		case i.op == list && j.op != list:
			return -1
		case i.op != list && j.op == list:
			return 1
		}

		switch {
		case resourceRanks[i.res] > resourceRanks[j.res]:
			return -1
		case resourceRanks[i.res] < resourceRanks[j.res]:
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
	})
	return merged
}

func lockQuery(query []Subquery, resultCount int) ([]Result, func(), error) {
	if len(query) == 0 {
		return nil, func() {}, nil
	}

	results := make([]Result, resultCount)
	unlocks := make([]func(), 0, len(query))

	// lock unique keys first
	uniqueKeysToLock := make(map[uniqueKeyToLock]bool, len(query))
	for i := range query {
		for _, k := range [2]string{query[i].key, query[i].newKey} {
			// no locks are needed for inconsistent reads, or if there is no key
			if k == "" || query[i].op == inconsistentRead {
				continue
			}
			uk := uniqueKeyToLock{
				r: query[i].res,
				k: k,
			}
			if !uniqueKeysToLock[uk] {
				uniqueKeysToLock[uk] = isWriteOp(query[i].op)
			}
		}
	}
	uniqueKeys := make([]uniqueKeyToLock, 0, len(uniqueKeysToLock))
	for k := range uniqueKeysToLock {
		uniqueKeys = append(uniqueKeys, k)
	}
	slices.SortFunc(uniqueKeys, func(i, j uniqueKeyToLock) int {
		switch {
		case resourceRanks[i.r] > resourceRanks[j.r]:
			return -1
		case resourceRanks[i.r] < resourceRanks[j.r]:
			return 1
		}

		switch {
		case i.r < j.r:
			return -1
		case i.r > j.r:
		}

		switch {
		case i.k < j.k:
			return -1
		case i.k > j.k:
			return 1
		}

		return 0
	})
	for _, uk := range uniqueKeys {
		c := caches[uk.r]
		if c.key == nil {
			continue
		}
		if uniqueKeysToLock[uk] {
			c.key.keyLocks.Lock(uk.k)
			unlocks = append(unlocks, func() {
				c.key.keyLocks.Unlock(uk.k)
			})
		} else {
			c.key.keyLocks.RLock(uk.k)
			unlocks = append(unlocks, func() {
				c.key.keyLocks.RUnlock(uk.k)
			})
		}
	}

	// lock IDs
	for i := range query {
		var previousQuery *Subquery
		if i > 0 {
			previousQuery = &query[i-1]
		}
		// no locks are needed for lists or inconsistent reads
		if query[i].op == list || query[i].op == inconsistentRead {
			continue
		}
		allowBlankID := caches[query[i].res].allowBlankID
		if query[i].id == "" && !allowBlankID {
			continue
		}
		unlock := lockSubquery(&query[i], previousQuery, allowBlankID)
		if unlock != nil {
			unlocks = append(unlocks, unlock)
		}
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

func lockSubquery(subquery, previousSubquery *Subquery, allowBlankID bool) func() {
	c := caches[subquery.res]
	var u func()
	// take a new lock only if the previous resource and id are different and the operation is a write and there is
	// an id to lock.
	if (subquery.id != "" || !allowBlankID) &&
		(previousSubquery == nil || previousSubquery.res != subquery.res || previousSubquery.id != subquery.id) {
		if isWriteOp(subquery.op) {
			c.resourceLocks.Lock(subquery.id)
			u = func() {
				c.resourceLocks.Unlock(subquery.id)
			}
		} else {
			c.resourceLocks.RLock(subquery.id)
			u = func() {
				c.resourceLocks.RUnlock(subquery.id)
			}
		}
	}
	return u
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
		return fmt.Errorf("expected one dependency, got %data", len(s[i].dependencies))
	}
	if s[s[i].dependencies[0]].res != r {
		return fmt.Errorf("expected %s dependency, got %s", r, s[s[i].dependencies[0]].res)
	}
	return nil
}

func rankForResource(r resource) int {
	rank := 0
	for _, c := range inverseSchema[r] {
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

	Node              NodeResult
	StorageClass      StorageClassResult
	Backend           BackendResult
	Volume            VolumeResult
	SubordinateVolume SubordinateVolumeResult
	VolumePublication VolumePublicationResult
	Snapshot          SnapshotResult
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

type resource int

func (r resource) String() string {
	return resourceNames[r]
}

var resourceNames = map[resource]string{
	node:              "Node",
	storageClass:      "Storage Class",
	backend:           "Backend",
	volume:            "Volume",
	subordinateVolume: "Subordinate Volume",
	volumePublication: "Volume Publication",
	snapshot:          "Snapshot",
}

const (
	node = resource(iota)
	storageClass
	backend
	volume
	subordinateVolume
	volumePublication
	snapshot

	resourceCount = int(snapshot) + 1
)

type operation int

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
	node:              nil,
	storageClass:      nil,
	backend:           nil,
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

type uniqueKeyToLock struct {
	r resource
	k string
}
