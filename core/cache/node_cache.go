package cache

import (
	"sync"

	"github.com/netapp/trident/utils"
)

type NodeCache struct {
	nodes map[string]*utils.Node
	m     *sync.RWMutex
}

func NewNodeCache() *NodeCache {
	return &NodeCache{
		nodes: make(map[string]*utils.Node),
		m:     &sync.RWMutex{},
	}
}

// Get returns a node by name, or nil if the node is not found
func (nc *NodeCache) Get(name string) *utils.Node {
	nc.m.RLock()
	defer nc.m.RUnlock()

	node, ok := nc.nodes[name]
	if !ok {
		return nil
	}
	return node.Copy()
}

// Set adds or updates node in cache. Set should only be used with the global lock.
func (nc *NodeCache) Set(name string, node *utils.Node) {
	nc.m.Lock()
	defer nc.m.Unlock()

	nc.nodes[name] = node
}

// Delete removes the node from cache. Does nothing if node does not exist. Delete should only be used with the global
// lock.
func (nc *NodeCache) Delete(name string) {
	nc.m.Lock()
	defer nc.m.Unlock()

	delete(nc.nodes, name)
}

// List returns nodes in cache as an unordered slice that is safe to modify.
func (nc *NodeCache) List() []*utils.Node {
	nc.m.RLock()
	defer nc.m.RUnlock()

	l := make([]*utils.Node, 0, len(nc.nodes))
	for _, v := range nc.nodes {
		l = append(l, v.Copy())
	}
	return l
}

// Len returns the number of nodes in cache
func (nc *NodeCache) Len() int {
	nc.m.RLock()
	defer nc.m.RUnlock()

	return len(nc.nodes)
}
