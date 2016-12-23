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
		c.Version = config.OrchestratorMajorVersion
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

func (s *StorageClass) Matches(vc *storage.StoragePool) bool {
	if len(s.config.BackendStoragePools) > 0 {
		if vcList, ok := s.config.BackendStoragePools[vc.Backend.Name]; ok {
			for _, vcName := range vcList {
				if vcName == vc.Name {
					return true
				}
			}
		}
	}
	matches := len(s.config.Attributes) > 0
	for name, request := range s.config.Attributes {
		if vc.Attributes == nil {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         vc.Name,
				"attribute":    name,
			}).Panic("Storage pool attributes are nil")
		}
		if offer, ok := vc.Attributes[name]; !ok || !offer.Matches(request) {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         vc.Name,
				"attribute":    name,
				"found":        ok}).Debug("Attribute for storage " +
				"pool failed to match storage class.")
			matches = false
			break
		}
	}
	return matches
}

// CheckAndAddBackend iterates through each of the storage pools
// for a given backend.  If the pool satisfies the storage class, it
// adds that pool.  Returns the number of storage pools added.
func (s *StorageClass) CheckAndAddBackend(b *storage.StorageBackend) int {
	if !b.Online {
		return 0
	}
	added := 0
	for _, vc := range b.Storage {
		if s.Matches(vc) {
			s.pools = append(s.pools, vc)
			vc.AddStorageClass(s.GetName())
			added++
		}
	}
	return added
}

func (s *StorageClass) RemovePoolsForBackend(backend *storage.StorageBackend) {
	newVCList := make([]*storage.StoragePool, 0)
	for _, vc := range s.pools {
		if vc.Backend != backend {
			newVCList = append(newVCList, vc)
		}
	}
	s.pools = newVCList
}

func (s *StorageClass) GetVolumes() []*storage.Volume {
	ret := make([]*storage.Volume, 0)
	for _, vc := range s.pools {
		for _, vol := range vc.Volumes {
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

func (s *StorageClass) GetBackendStoragePools() map[string][]string {
	return s.config.BackendStoragePools
}

func (s *StorageClass) GetStoragePoolsForProtocol(p config.Protocol) []*storage.StoragePool {
	ret := make([]*storage.StoragePool, 0, len(s.pools))
	// TODO:  Change this to work with indices of backends?
	for _, vc := range s.pools {
		if p == config.ProtocolAny || vc.Backend.GetProtocol() == p {
			ret = append(ret, vc)
		}
	}
	return ret
}

func (s *StorageClass) ConstructExternal() *StorageClassExternal {
	ret := &StorageClassExternal{
		Config:       s.config,
		StoragePools: make(map[string][]string),
	}
	for _, vc := range s.pools {
		backendName := vc.Backend.Name
		if vcList, ok := ret.StoragePools[backendName]; ok {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         vc.Name,
				"Backend":      backendName,
				"Method":       "ConstructExternal",
			}).Debug("Appending to existing storage pool list for backend.")
			ret.StoragePools[backendName] = append(vcList, vc.Name)
		} else {
			log.WithFields(log.Fields{
				"storageClass": s.GetName(),
				"pool":         vc.Name,
				"Backend":      backendName,
				"Method":       "ConstructExternal",
			}).Debug("Creating new storage pool list for backend.")
			ret.StoragePools[backendName] = make([]string, 1, 1)
			ret.StoragePools[backendName][0] = vc.Name
		}
	}
	for _, list := range ret.StoragePools {
		sort.Strings(list)
	}
	for _, list := range ret.Config.BackendStoragePools {
		sort.Strings(list)
	}
	return ret
}

func (s *StorageClassExternal) GetName() string {
	return s.Config.Name
}

func (s *StorageClass) ConstructPersistent() *StorageClassPersistent {
	ret := &StorageClassPersistent{Config: s.config}
	for _, list := range ret.Config.BackendStoragePools {
		sort.Strings(list)
	}
	return ret
}

func (s *StorageClassPersistent) GetName() string {
	return s.Config.Name
}
