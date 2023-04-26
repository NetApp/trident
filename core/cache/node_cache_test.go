package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils"
)

func TestNodeCacheSetAndGet(t *testing.T) {
	node1 := &utils.Node{
		Name: "node1",
	}
	node2 := &utils.Node{
		Name: "node2",
	}
	cache := NewNodeCache()
	cache.Set(node1.Name, node1)
	cache.Set(node2.Name, node2)

	assert.Equal(t, node1, cache.Get(node1.Name))
	assert.Equal(t, node2, cache.Get(node2.Name))
	assert.Nil(t, cache.Get("node3"))
}

func TestNodeCacheLen(t *testing.T) {
	node1 := &utils.Node{
		Name: "node1",
	}
	cache := NewNodeCache()
	assert.Equal(t, 0, cache.Len())

	cache.Set(node1.Name, node1)
	assert.Equal(t, 1, cache.Len())
}

func TestNodeCacheList(t *testing.T) {
	node1 := &utils.Node{
		Name: "node1",
	}
	node2 := &utils.Node{
		Name: "node2",
	}
	cache := NewNodeCache()
	assert.Empty(t, cache.List())

	cache.Set(node1.Name, node1)
	cache.Set(node2.Name, node2)
	assert.ElementsMatch(t, []*utils.Node{node1, node2}, cache.List())
}

// TestNodeCacheDelete tests delete removes entry and is idempotent
func TestNodeCacheDelete(t *testing.T) {
	node1 := &utils.Node{
		Name: "node1",
	}
	cache := NewNodeCache()

	cache.Set(node1.Name, node1)
	assert.Equal(t, 1, cache.Len())
	cache.Delete(node1.Name)
	assert.Zero(t, cache.Len())
	cache.Delete(node1.Name)
	assert.Zero(t, cache.Len())
}

// TestNodeCacheGetUnblocked tests multiple reads are not blocked
// should be used with -race test option
// 1. Get read lock
// 2. Get node
// 3. See no deadlock
func TestNodeCacheGetUnblocked(t *testing.T) {
	node1 := &utils.Node{
		Name: "node1",
	}
	cache := NewNodeCache()
	cache.Set(node1.Name, node1)

	cache.m.RLock()
	defer cache.m.RUnlock()

	node := cache.Get(node1.Name)
	assert.Equal(t, node1, node)
}

func TestNodeCacheAccessorsWaitForWriteLock(t *testing.T) {
	const (
		holdDuration   = 100 * time.Millisecond
		waitedAtLeast  = 50 * time.Millisecond
		ensureLockHeld = 10 * time.Millisecond
	)
	holdLock := func(cache *NodeCache) {
		cache.m.Lock()
		time.Sleep(holdDuration)
		cache.m.Unlock()
	}

	tests := []struct {
		name     string
		accessor func(cache *NodeCache, node *utils.Node)
	}{
		{
			name: "Get",
			accessor: func(cache *NodeCache, node *utils.Node) {
				_ = cache.Get(node.Name)
			},
		},
		{
			name: "Len",
			accessor: func(cache *NodeCache, node *utils.Node) {
				_ = cache.Len()
			},
		},
		{
			name: "List",
			accessor: func(cache *NodeCache, node *utils.Node) {
				_ = cache.List()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node1 := &utils.Node{
				Name: "node1",
			}
			cache := NewNodeCache()
			cache.Set(node1.Name, node1)

			start := time.Now()
			// hold write lock for some time
			go holdLock(cache)
			// ensure write lock is held
			time.Sleep(ensureLockHeld)
			// run mutator
			test.accessor(cache, node1)
			assert.True(t, time.Now().After(start.Add(waitedAtLeast)))
		})
	}
}

func TestNodeCacheMutatorsWaitForReadLock(t *testing.T) {
	const (
		holdDuration   = 100 * time.Millisecond
		waitedAtLeast  = 50 * time.Millisecond
		ensureLockHeld = 10 * time.Millisecond
	)
	holdRLock := func(cache *NodeCache) {
		cache.m.RLock()
		time.Sleep(holdDuration)
		cache.m.RUnlock()
	}

	tests := []struct {
		name    string
		mutator func(cache *NodeCache, node *utils.Node)
	}{
		{
			name: "Set",
			mutator: func(cache *NodeCache, node *utils.Node) {
				cache.Set(node.Name, node)
			},
		},
		{
			name: "Delete",
			mutator: func(cache *NodeCache, node *utils.Node) {
				cache.Delete(node.Name)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node1 := &utils.Node{
				Name: "node1",
			}
			cache := NewNodeCache()

			start := time.Now()
			// hold read lock for some time
			go holdRLock(cache)
			// ensure read lock is held
			time.Sleep(ensureLockHeld)
			// run mutator
			test.mutator(cache, node1)
			assert.True(t, time.Now().After(start.Add(waitedAtLeast)))
		})
	}
}
