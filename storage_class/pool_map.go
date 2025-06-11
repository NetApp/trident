// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
)

type poolMap map[string]map[string][]string

// PoolMap is a map of storage class names to backend names to pool names.
// This is used to track the storage class to pool mapping for each backend.
// Example:
//
//	{
//	  "storageClassA":{
//	     "backendA":[
//	        "pool1",
//	        "pool2"
//	     ],
//	     "backendB":[
//	        "pool3",
//	        "pool4"
//	     ]
//	  },
//	  "storageClassB":{
//	     "backendA":[
//	        "pool1"
//	     ]
//	  }
//	}
type PoolMap struct {
	poolMap
}

// NewPoolMap creates a new empty PoolMap.
func NewPoolMap() *PoolMap {
	return &PoolMap{make(poolMap)}
}

// DeepCopy creates a deep copy of the PoolMap.
func (m *PoolMap) DeepCopy() *PoolMap {
	newMap := make(poolMap, len(m.poolMap))

	for key, value := range m.poolMap {
		newMap[key] = make(map[string][]string, len(value))
		for innerKey, innerValue := range value {
			newMap[key][innerKey] = append([]string(nil), innerValue...)
		}
	}

	return &PoolMap{newMap}
}

// Rebuild rebuilds the PoolMap based on the provided storage classes and backends.
func (m *PoolMap) Rebuild(
	ctx context.Context, storageClasses []*StorageClass, backends []storage.Backend,
) {
	newMap := make(poolMap, len(storageClasses))

	for _, sc := range storageClasses {
		if _, ok := newMap[sc.GetName()]; !ok {
			newMap[sc.GetName()] = make(map[string][]string)
		}

		for _, b := range backends {
			if !b.State().IsOnline() {
				Logc(ctx).WithField("backend", b.Name()).Debug("Backend not online.")
				continue
			}

			b.StoragePools().Range(func(_, v interface{}) bool {
				pool := v.(storage.Pool)
				if sc.Matches(ctx, pool) {
					newMap[sc.GetName()][b.Name()] = append(newMap[sc.GetName()][b.Name()], pool.Name())
				}
				return true
			})
		}
	}

	m.poolMap = newMap
}

// BackendPoolMapForStorageClass returns a map of backend names to pool names for a given storage class.
func (m *PoolMap) BackendPoolMapForStorageClass(ctx context.Context, scName string) map[string][]string {
	if pools, ok := m.poolMap[scName]; ok {
		return pools
	}
	Logc(ctx).WithField("storageClass", scName).Debug("Storage class not found.")
	return nil
}

// StorageClassNamesForPoolName returns a list of storage class names for a given backend and pool name.
func (m *PoolMap) StorageClassNamesForPoolName(_ context.Context, backendName, poolName string) []string {
	var scNames []string

	for scName, backendPoolMap := range m.poolMap {
		if poolNames, ok := backendPoolMap[backendName]; ok {
			for _, pName := range poolNames {
				if pName == poolName {
					scNames = append(scNames, scName)
				}
			}
		}
	}

	return scNames
}

// StorageClassNamesForBackendName returns a list of storage class names for each of a backend's pools.
// The returned value is a map of lists of storage class names, keyed by pool name.
func (m *PoolMap) StorageClassNamesForBackendName(ctx context.Context, backendName string) map[string][]string {
	scNames := make(map[string][]string)

	for scName, backendPoolMap := range m.poolMap {
		if poolNames, ok := backendPoolMap[backendName]; ok {
			for _, pName := range poolNames {
				scNames[pName] = append(scNames[pName], scName)
			}
		}
	}

	if len(scNames) == 0 {
		Logc(ctx).WithField("backend", backendName).Debug("No storage classes found.")
	}

	return scNames
}

func (m *PoolMap) BackendMatchesStorageClass(_ context.Context, backendName, storageClassName string) bool {
	matchingBackendPools, ok := m.poolMap[storageClassName]
	if !ok {
		return false
	}

	if pool := matchingBackendPools[backendName]; pool != nil {
		// If the backend is in the map and has pools, then it matches.
		if len(pool) > 0 {
			return true
		}
	}

	return false
}
