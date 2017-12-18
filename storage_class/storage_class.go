// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_class

import (
	"encoding/json"
	"fmt"
	"sort"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
)

func New(c *Config) *StorageClass {
	if c.Version == "" {
		c.Version = config.OrchestratorAPIVersion
	}
	return &StorageClass{
		config: c,
		pools:  make([]*storage.StoragePool, 0),
	}
}

func NewForConfig(configJSON string) (*StorageClass, error) {
	var config Config
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("Unable to unmarshal config:  %v", err)
	}
	return New(&config), nil
}

func NewFromPersistent(persistent *StorageClassPersistent) *StorageClass {
	return New(persistent.Config)
}

func (s *StorageClass) Matches(storagePool *storage.StoragePool) bool {

	// Check additionalStoragePools first, since it can yield a match result by itself
	if len(s.config.AdditionalPools) > 0 {
		if storagePoolList, ok := s.config.AdditionalPools[storagePool.Backend.Name]; ok {
			for _, storagePoolName := range storagePoolList {
				if storagePoolName == storagePool.Name {
					log.WithFields(log.Fields{
						"storageClass": s.GetName(),
						"pool":         storagePool.Name,
					}).Debug("Matched by additionalStoragePools attribute.")
					return true
				}
			}
		}

		// Handle the sub-case where additionalStoragePools is specified (but didn't match) and
		// there are no attributes or storagePools specified in the storage class.  This should
		// always return false.
		if len(s.config.Attributes) == 0 && len(s.config.Pools) == 0 {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         storagePool.Name,
			}).Debug("Pool failed to match storage class additionalStoragePools attribute.")
			return false
		}
	}

	// Attributes are used to narrow the pool selection.  Therefore if no attributes are
	// specified, then all pools can match.  If one or more attributes are specified in the
	// storage class, then all must match.
	attributesMatch := true
	for name, request := range s.config.Attributes {
		if offer, ok := storagePool.Attributes[name]; !ok || !offer.Matches(request) {
			log.WithFields(log.Fields{
				"offer":        offer,
				"request":      request,
				"storageClass": s.GetName(),
				"pool":         storagePool.Name,
				"attribute":    name,
				"found":        ok,
			}).Debug("Attribute for storage pool failed to match storage class.")
			attributesMatch = false
			break
		}
	}

	// The storagePools list is used to narrow the pool selection.  Therefore if no pools are
	// specified, then all pools can match.  If one or more pools are listed in the storage
	// class, then the pool must be in the list.
	poolsMatch := true
	if len(s.config.Pools) > 0 {
		poolsMatch = false
		if storagePoolList, ok := s.config.Pools[storagePool.Backend.Name]; ok {
			for _, storagePoolName := range storagePoolList {
				if storagePoolName == storagePool.Name {
					poolsMatch = true
				}
			}
		}
	}

	result := attributesMatch && poolsMatch

	log.WithFields(log.Fields{
		"attributesMatch": attributesMatch,
		"poolsMatch":      poolsMatch,
		"match":           result,
		"pool":            storagePool.Name,
		"storageClass":    s.GetName(),
	}).Debug("Result of pool match for storage class.")

	return result
}

// CheckAndAddBackend iterates through each of the storage pools
// for a given backend.  If the pool satisfies the storage class, it
// adds that pool.  Returns the number of storage pools added.
func (s *StorageClass) CheckAndAddBackend(b *storage.StorageBackend) int {

	log.WithFields(log.Fields{
		"backend":      b.Name,
		"storageClass": s.GetName(),
	}).Debug("Checking backend for storage class")

	if !b.Online {
		log.WithField("backend", b.Name).Warn("Backend not online.")
		return 0
	}

	added := 0
	for _, storagePool := range b.Storage {
		if s.Matches(storagePool) {
			s.pools = append(s.pools, storagePool)
			storagePool.AddStorageClass(s.GetName())
			added++
			log.WithFields(log.Fields{
				"pool":         storagePool.Name,
				"storageClass": s.GetName(),
			}).Debug("Storage class added to the storage pool.")
		}
	}
	return added
}

func (s *StorageClass) RemovePoolsForBackend(backend *storage.StorageBackend) {
	newStoragePools := make([]*storage.StoragePool, 0)
	for _, storagePool := range s.pools {
		if storagePool.Backend != backend {
			newStoragePools = append(newStoragePools, storagePool)
		}
	}
	s.pools = newStoragePools
}

func (s *StorageClass) GetVolumes() []*storage.Volume {
	ret := make([]*storage.Volume, 0)
	for _, storagePool := range s.pools {
		for _, vol := range storagePool.Volumes {
			// Because storage pools can fulfill more than one storage
			// class, we have to check each volume for that pool to see
			// if it uses this storage class.
			if vol.Config.StorageClass == s.GetName() {
				ret = append(ret, vol)
			}
		}
	}
	return ret
}

func (s *StorageClass) GetAttributes() map[string]storage_attribute.Request {
	return s.config.Attributes
}

func (s *StorageClass) GetName() string {
	return s.config.Name
}

func (s *StorageClass) GetStoragePools() map[string][]string {
	return s.config.Pools
}

func (s *StorageClass) GetAdditionalStoragePools() map[string][]string {
	return s.config.AdditionalPools
}

func (s *StorageClass) GetStoragePoolsForProtocol(p config.Protocol) []*storage.StoragePool {
	ret := make([]*storage.StoragePool, 0, len(s.pools))
	// TODO:  Change this to work with indices of backends?
	for _, storagePool := range s.pools {
		if p == config.ProtocolAny || storagePool.Backend.GetProtocol() == p {
			ret = append(ret, storagePool)
		}
	}
	return ret
}

func (s *StorageClass) ConstructExternal() *StorageClassExternal {
	ret := &StorageClassExternal{
		Config:       s.config,
		StoragePools: make(map[string][]string),
	}
	for _, storagePool := range s.pools {
		backendName := storagePool.Backend.Name
		if storagePoolList, ok := ret.StoragePools[backendName]; ok {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         storagePool.Name,
				"Backend":      backendName,
				"Method":       "ConstructExternal",
			}).Debug("Appending to existing storage pool list for backend.")
			ret.StoragePools[backendName] = append(storagePoolList, storagePool.Name)
		} else {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         storagePool.Name,
				"Backend":      backendName,
				"Method":       "ConstructExternal",
			}).Debug("Creating new storage pool list for backend.")
			ret.StoragePools[backendName] = make([]string, 1, 1)
			ret.StoragePools[backendName][0] = storagePool.Name
		}
	}
	for _, list := range ret.StoragePools {
		sort.Strings(list)
	}
	for _, list := range ret.Config.Pools {
		sort.Strings(list)
	}
	for _, list := range ret.Config.AdditionalPools {
		sort.Strings(list)
	}
	return ret
}

func (s *StorageClassExternal) GetName() string {
	return s.Config.Name
}

func (s *StorageClass) ConstructPersistent() *StorageClassPersistent {
	ret := &StorageClassPersistent{Config: s.config}
	for _, list := range ret.Config.Pools {
		sort.Strings(list)
	}
	for _, list := range ret.Config.AdditionalPools {
		sort.Strings(list)
	}
	return ret
}

func (s *StorageClassPersistent) GetName() string {
	return s.Config.Name
}
