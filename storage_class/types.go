// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_class

import (
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
)

type StorageClass struct {
	config *Config
	pools  []*storage.StoragePool
}

type Config struct {
	//NOTE:  Ensure that any changes made to this data structure are reflected
	// in the Unmarshal method of config.go
	Version             string                               `json:"version"`
	Name                string                               `json:"name"`
	Attributes          map[string]storage_attribute.Request `json:"attributes,omitempty"`
	BackendStoragePools map[string][]string                  `json:"requiredStorage,omitempty"`
}

type StorageClassExternal struct {
	Config       *Config
	StoragePools map[string][]string `json:"storage"` // Backend -> list of StoragePools
}

// StorageClassPersistent contains the minimal information needed to persist
// a StorageClass.  This exists to give us some flexibility to evolve the
// struct; it also avoids overloading the semantics of Config and is
// consistent with StorageBackendExternal.
type StorageClassPersistent struct {
	Config *Config `json:"config"`
}
