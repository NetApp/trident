// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
)

type StorageClass struct {
	config *Config
	pools  []storage.Pool
}

type Config struct {
	// NOTE:  Ensure that any changes made to this data structure are reflected
	// in the Unmarshal method of config.go
	Version         string                              `json:"version" hash:"ignore"`
	Name            string                              `json:"name" hash:"ignore"`
	Attributes      map[string]storageattribute.Request `json:"attributes,omitempty"`
	Pools           map[string][]string                 `json:"storagePools,omitempty"`
	AdditionalPools map[string][]string                 `json:"additionalStoragePools,omitempty"`
	ExcludePools    map[string][]string                 `json:"excludeStoragePools,omitempty"`
	AutogrowPolicy  string                              `json:"autogrowPolicy,omitempty"`
}

type External struct {
	Config       *Config
	StoragePools map[string][]string `json:"storage"` // Backend -> list of StoragePools
}

// Persistent contains the minimal information needed to persist
// a StorageClass.  This exists to give us some flexibility to evolve the
// struct; it also avoids overloading the semantics of Config and is
// consistent with BackendExternal.
type Persistent struct {
	Config *Config `json:"config"`
}
