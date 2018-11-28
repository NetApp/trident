// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

import (
	"sort"

	sa "github.com/netapp/trident/storage_attribute"
)

type Pool struct {
	Name string
	// A Trident storage pool can potentially satisfy more than one storage
	// class.
	StorageClasses []string
	Backend        *Backend
	Attributes     map[string]sa.Offer
}

func NewStoragePool(backend *Backend, name string) *Pool {
	return &Pool{
		Name:           name,
		StorageClasses: make([]string, 0),
		Backend:        backend,
		Attributes:     make(map[string]sa.Offer),
	}
}

func (pool *Pool) AddStorageClass(class string) {
	// Note that this function should get called once per storage class
	// affecting the volume; thus, we don't need to check for duplicates.
	pool.StorageClasses = append(pool.StorageClasses, class)
}

func (pool *Pool) RemoveStorageClass(class string) bool {
	found := false
	for i, name := range pool.StorageClasses {
		if name == class {
			pool.StorageClasses = append(pool.StorageClasses[:i],
				pool.StorageClasses[i+1:]...)
			found = true
			break
		}
	}
	return found
}

type PoolExternal struct {
	Name           string   `json:"name"`
	StorageClasses []string `json:"storageClasses"`
	//TODO: can't have an interface here for unmarshalling
	Attributes map[string]sa.Offer `json:"storageAttributes"`
}

func (pool *Pool) ConstructExternal() *PoolExternal {
	external := &PoolExternal{
		Name:           pool.Name,
		StorageClasses: pool.StorageClasses,
		Attributes:     pool.Attributes,
	}

	// We want to sort these so that the output remains consistent;
	// there are cases where the order won't always be the same.
	sort.Strings(external.StorageClasses)
	return external
}
