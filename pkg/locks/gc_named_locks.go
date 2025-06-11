package locks

import "sync"

// GCNamedMutex provides garbage-collected named RW mutexes.
type GCNamedMutex struct {
	mutexes map[string]*gcMutex
	m       *sync.Mutex
}

type gcMutex struct {
	m sync.RWMutex
	c int
}

func NewGCNamedMutex() *GCNamedMutex {
	return &GCNamedMutex{
		mutexes: make(map[string]*gcMutex),
		m:       &sync.Mutex{},
	}
}

func (g *GCNamedMutex) Lock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		resourceMutex = &gcMutex{}
		g.mutexes[name] = resourceMutex
	}
	resourceMutex.c++
	g.m.Unlock()

	resourceMutex.m.Lock()
}

func (g *GCNamedMutex) Unlock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		g.m.Unlock()
		return
	}
	resourceMutex.c--
	if resourceMutex.c == 0 {
		delete(g.mutexes, name)
	}
	g.m.Unlock()

	resourceMutex.m.Unlock()
}

func (g *GCNamedMutex) RLock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		resourceMutex = &gcMutex{}
		g.mutexes[name] = resourceMutex
	}
	resourceMutex.c++
	g.m.Unlock()

	resourceMutex.m.RLock()
}

func (g *GCNamedMutex) RUnlock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		g.m.Unlock()
		return
	}
	resourceMutex.c--
	if resourceMutex.c == 0 {
		delete(g.mutexes, name)
	}
	g.m.Unlock()

	resourceMutex.m.RUnlock()
}
