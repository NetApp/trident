// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
	"sort"

	sa "github.com/netapp/trident/storage_attribute"
)

type StoragePool struct {
	Name string
	// A Trident storage pool can potentially satisfy more than one storage
	// class.
	StorageClasses []string
	Volumes        map[string]*Volume
	Backend        *StorageBackend
	Attributes     map[string]sa.Offer
}

func NewStoragePool(backend *StorageBackend, name string) *StoragePool {
	return &StoragePool{
		Name:           name,
		StorageClasses: make([]string, 0),
		Volumes:        make(map[string]*Volume),
		Backend:        backend,
		Attributes:     make(map[string]sa.Offer),
	}
}

func (pool *StoragePool) AddVolume(vol *Volume, bootstrap bool) {
	pool.Volumes[vol.Config.Name] = vol
}

func (pool *StoragePool) DeleteVolume(vol *Volume) bool {
	if _, ok := pool.Volumes[vol.Config.Name]; ok {
		delete(pool.Volumes, vol.Config.Name)
		return true
	}
	return false
}

func (pool *StoragePool) AddStorageClass(class string) {
	// Note that this function should get called once per storage class
	// affecting the volume; thus, we don't need to check for duplicates.
	pool.StorageClasses = append(pool.StorageClasses, class)
}

func (pool *StoragePool) RemoveStorageClass(class string) bool {
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

type StoragePoolExternal struct {
	Name           string   `json:"name"`
	StorageClasses []string `json:"storageClasses"`
	//TODO: can't have an interface here for unmarshalling
	Attributes map[string]sa.Offer `json:"storageAttributes"`
	Volumes    []string            `json:"volumes"`
}

func (pool *StoragePool) ConstructExternal() *StoragePoolExternal {
	external := &StoragePoolExternal{
		Name:           pool.Name,
		StorageClasses: pool.StorageClasses,
		Attributes:     make(map[string]sa.Offer),
		Volumes:        make([]string, 0, len(pool.Volumes)),
	}
	for k, v := range pool.Attributes {
		external.Attributes[k] = v
	}
	for name, _ := range pool.Volumes {
		external.Volumes = append(external.Volumes, name)
	}
	// We want to sort these so that the output remains consistent;
	// there are cases where the order won't always be the same.
	sort.Strings(external.StorageClasses)
	sort.Strings(external.Volumes)
	return external
}
