// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	. "github.com/netapp/trident/logger"
	sa "github.com/netapp/trident/storage_attribute"
)

type Pool struct {
	Name string
	// A Trident storage pool can potentially satisfy more than one storage class.
	StorageClasses      []string
	Backend             *Backend
	Attributes          map[string]sa.Offer // These attributes are used to match storage classes
	InternalAttributes  map[string]string   // These attributes are defined & used internally by storage drivers
	SupportedTopologies []map[string]string
}

func NewStoragePool(backend *Backend, name string) *Pool {
	return &Pool{
		Name:               name,
		StorageClasses:     make([]string, 0),
		Backend:            backend,
		Attributes:         make(map[string]sa.Offer),
		InternalAttributes: make(map[string]string),
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
	Attributes          map[string]sa.Offer `json:"storageAttributes"`
	SupportedTopologies []map[string]string `json:"supportedTopologies"`
}

func (pool *Pool) ConstructExternal() *PoolExternal {
	external := &PoolExternal{
		Name:                pool.Name,
		StorageClasses:      pool.StorageClasses,
		Attributes:          pool.Attributes,
		SupportedTopologies: pool.SupportedTopologies,
	}

	// We want to sort these so that the output remains consistent;
	// there are cases where the order won't always be the same.
	sort.Strings(external.StorageClasses)
	return external
}

// GetLabelsJSON returns a JSON-formatted string containing the labels on this pool, suitable
// for a label set on a storage volume.  The outer key may be customized.  For example:
// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}
func (pool *Pool) GetLabelsJSON(ctx context.Context, key string) string {

	labelOffer, ok := pool.Attributes[sa.Labels].(sa.LabelOffer)
	if !ok {
		return ""
	}

	labelOfferMap := labelOffer.Labels()
	if len(labelOfferMap) == 0 {
		return ""
	}

	poolLabelMap := make(map[string]map[string]string)
	poolLabelMap[key] = labelOfferMap

	poolLabelJSON, err := json.Marshal(poolLabelMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal pool labels: %+v", poolLabelMap)
		return ""
	}

	return strings.ReplaceAll(string(poolLabelJSON), " ", "")
}
