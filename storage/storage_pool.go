// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	drivers "github.com/netapp/trident/v21/storage_drivers"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/v21/logger"
	sa "github.com/netapp/trident/v21/storage_attribute"
)

// TODO: Try moving all ProvisioningLabelTag related code here
const ProvisioningLabelTag = "provisioning"

type Pool struct {
	Name string
	// A Trident storage pool can potentially satisfy more than one storage class.
	StorageClasses      []string
	Backend             Backend
	Attributes          map[string]sa.Offer // These attributes are used to match storage classes
	InternalAttributes  map[string]string   // These attributes are defined & used internally by storage drivers
	SupportedTopologies []map[string]string
}

func NewStoragePool(backend Backend, name string) *Pool {
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
func (pool *Pool) GetLabelsJSON(ctx context.Context, key string, labelLimit int) (string, error) {

	labelOffer, ok := pool.Attributes[sa.Labels].(sa.LabelOffer)
	if !ok {
		return "", nil
	}

	labelOfferMap := labelOffer.Labels()
	if len(labelOfferMap) == 0 {
		return "", nil
	}

	poolLabelMap := make(map[string]map[string]string)
	poolLabelMap[key] = labelOfferMap

	poolLabelJSON, err := json.Marshal(poolLabelMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal pool labels: %+v", poolLabelMap)
		return "", err
	}

	labelsJsonBytes := new(bytes.Buffer)
	err = json.Compact(labelsJsonBytes, poolLabelJSON)
	if err != nil {
		Logc(ctx).Errorf("Failed to compact pool labels: %s", string(poolLabelJSON))
		return "", err
	}

	labelsJSON := labelsJsonBytes.String()

	if labelLimit != 0 && len(labelsJSON) > labelLimit {
		Logc(ctx).WithFields(log.Fields{
			"labelsJSON":       labelsJSON,
			"labelsJSONLength": len(labelsJSON),
			"maxLabelLength":   labelLimit}).Error("label length exceeds the character limit")
		return "", fmt.Errorf("label length %v exceeds the character limit of %v characters", len(labelsJSON),
			labelLimit)
	}

	return labelsJSON, nil
}

// AllowLabelOverwrite returns true if it has a key we could have set. For example:
// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}
func AllowPoolLabelOverwrite(key, originalLabel string) bool {

	if originalLabel == "" {
		return false
	}

	var poolLabelMap map[string]map[string]string
	err := json.Unmarshal([]byte(originalLabel), &poolLabelMap)
	if err != nil {
		// this key is not in our format and hence it is set by another product
		// so it is readonly for us
		return false
	}
	if _, ok := poolLabelMap[key]; ok {
		// this key is in our format so it is set by us
		// OK to overwrite
		return true
	}
	return false
}

// updateProvisioningLabels returns the volume labels with an updated provisioning label provided
// Note:- Currently not used. Will be used for update storagevolume labels
func UpdateProvisioningLabels(provisioningLabel string, volumeLabels []string) []string {

	newLabels := DeleteProvisioningLabels(volumeLabels)
	return append(newLabels, provisioningLabel)
}

// deleteProvisioningLabels returns the volume labels with the provisioning label deleted
func DeleteProvisioningLabels(volumeLabels []string) []string {

	newLabels := make([]string, 0)

	for _, label := range volumeLabels {
		if !AllowPoolLabelOverwrite(ProvisioningLabelTag, label) {
			newLabels = append(newLabels, label)
		}
	}

	return newLabels
}

func IsStoragePoolUnset(storagePool *Pool) bool {
	return storagePool == nil || storagePool.Name == drivers.UnsetPool
}
