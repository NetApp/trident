// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cache

import (
	"fmt"
	"sync"

	"github.com/netapp/trident/utils/models"
)

type VolumePublicationCache struct {
	l *sync.RWMutex
	m map[string]map[string]*models.VolumePublication
}

// NewVolumePublicationCache returns a new reference to a VolumePublicationCache.
func NewVolumePublicationCache() *VolumePublicationCache {
	return &VolumePublicationCache{
		l: &sync.RWMutex{},
		m: make(map[string]map[string]*models.VolumePublication),
	}
}

// Set adds or updates an entry in the cache. If no keys or value is supplied, it errors.
// This expects the core's global lock is held.
func (vpc *VolumePublicationCache) Set(volume, node string, value *models.VolumePublication) error {
	vpc.l.Lock()
	defer vpc.l.Unlock()

	if volume == "" || node == "" {
		return fmt.Errorf("invalid keys supplied")
	}

	if value == nil {
		return fmt.Errorf("invalid value supplied")
	}

	// If the volume has no entry we need to initialize the inner map.
	if vpc.m[volume] == nil {
		vpc.m[volume] = map[string]*models.VolumePublication{}
	}

	vpc.m[volume][node] = value.Copy()
	return nil
}

// Get returns a copy of a value for an entry in the cache.
// Follow this with Set if changes to the returned value must exist in the cache.
func (vpc *VolumePublicationCache) Get(volume, node string) *models.VolumePublication {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	if volume == "" || node == "" {
		return nil
	}

	entry := vpc.m[volume][node]
	if entry == nil {
		return nil
	}

	return entry.Copy()
}

// TryGet attempts to retrieve an entry from the cache. If a value is found, a copy is returned.
// Callers should treat returned values as READ-ONLY.
// Follow this with Set if changes to the returned value must exist in the cache.
func (vpc *VolumePublicationCache) TryGet(volume, node string) (*models.VolumePublication, bool) {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	if volume == "" || node == "" {
		return nil, false
	}

	value, found := vpc.m[volume][node]
	if !found {
		return nil, false
	}

	return value.Copy(), found
}

// Delete removes an entry from the cache. If no node to publications exist for that entry, that structure is also removed.
// This expects the core's global lock is held.
func (vpc *VolumePublicationCache) Delete(volume, node string) error {
	vpc.l.Lock()
	defer vpc.l.Unlock()

	if volume == "" || node == "" {
		return fmt.Errorf("invalid keys supplied")
	}
	delete(vpc.m[volume], node)

	// If there are no publications for the volume, remove the node entry.
	if len(vpc.m[volume]) == 0 {
		delete(vpc.m, volume)
	}
	return nil
}

// ListPublications returns all publications.
// Callers should treat returned values as READ-ONLY.
// Follow this with SetMap if changes to the returned values must exist in the cache.
func (vpc *VolumePublicationCache) ListPublications() []*models.VolumePublication {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	publications := make([]*models.VolumePublication, 0)
	for _, nodeToPublications := range vpc.m {
		for _, publication := range nodeToPublications {
			if publication != nil {
				publications = append(publications, publication.Copy())
			}
		}
	}

	return publications
}

// ListPublicationsForVolume returns all publications for a volume.
// Callers should treat returned values as READ-ONLY.
// Follow this with SetMap if changes to the returned values must exist in the cache.
func (vpc *VolumePublicationCache) ListPublicationsForVolume(volume string) []*models.VolumePublication {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	publications := make([]*models.VolumePublication, 0)
	if volume != "" && vpc.m[volume] != nil {
		// Case where volume is supplied but not node;
		// return only publications relative to a volume.
		for _, publication := range vpc.m[volume] {
			publications = append(publications, publication.Copy())
		}
	}

	return publications
}

// ListPublicationsForNode returns all publications on a node.
// Callers should treat returned values as READ-ONLY.
// Follow this with SetMap if changes to the returned values must exist in the cache.
func (vpc *VolumePublicationCache) ListPublicationsForNode(node string) []*models.VolumePublication {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	publications := make([]*models.VolumePublication, 0)
	for _, nodeToPublications := range vpc.m {
		if nodeToPublications[node] != nil {
			publication := nodeToPublications[node]
			publications = append(publications, publication.Copy())
		}
	}

	return publications
}

// Map returns a copy of the internal map. If a slice is needed, use List instead.
// Follow this with SetMap if changes to the returned values must exist in the cache.
func (vpc *VolumePublicationCache) Map() map[string]map[string]*models.VolumePublication {
	vpc.l.RLock()
	defer vpc.l.RUnlock()

	return vpc.copyPublicationMap(vpc.m)
}

// SetMap sets the internal map to the supplied data.
// If no data is supplied, the internal map is initialized to an empty map.
// Any data present in the map before is lost and only the new data is stored.
// This expects the core's global lock is held.
func (vpc *VolumePublicationCache) SetMap(data map[string]map[string]*models.VolumePublication) {
	vpc.l.Lock()
	defer vpc.l.Unlock()

	vpc.m = vpc.copyPublicationMap(data)
}

// Clear wipes all data from the internal map.
// This expects the core's global lock is held.
func (vpc *VolumePublicationCache) Clear() {
	vpc.l.Lock()
	defer vpc.l.Unlock()

	vpc.m = make(map[string]map[string]*models.VolumePublication)
}

// copyPublicationMap copies the supplied map of data with new space and copies of the values.
func (vpc *VolumePublicationCache) copyPublicationMap(src map[string]map[string]*models.VolumePublication) map[string]map[string]*models.VolumePublication {
	dst := make(map[string]map[string]*models.VolumePublication, len(src))
	for volume, nodeToPublication := range src {
		if dst[volume] == nil {
			dst[volume] = map[string]*models.VolumePublication{}
		}
		for node, publication := range nodeToPublication {
			dst[volume][node] = publication.Copy()
		}
	}
	return dst
}
