// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	drivers "github.com/netapp/trident/storage_drivers"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	sa "github.com/netapp/trident/storage_attribute"
)

// TODO: Try moving all ProvisioningLabelTag related code here
const ProvisioningLabelTag = "provisioning"

type StoragePool struct {
	name string
	// A Trident storage pool can potentially satisfy more than one storage class.
	storageClasses      []string
	backend             Backend
	attributes          map[string]sa.Offer // These attributes are used to match storage classes
	internalAttributes  map[string]string   // These attributes are defined & used internally by storage drivers
	supportedTopologies []map[string]string
}

func (p *StoragePool) Name() string {
	return p.name
}

func (p *StoragePool) SetName(name string) {
	p.name = name
}

func (p *StoragePool) StorageClasses() []string {
	return p.storageClasses
}

func (p *StoragePool) SetStorageClasses(storageClasses []string) {
	p.storageClasses = storageClasses
}

func (p *StoragePool) Backend() Backend {
	return p.backend
}

func (p *StoragePool) SetBackend(backend Backend) {
	p.backend = backend
}

func (p *StoragePool) Attributes() map[string]sa.Offer {
	return p.attributes
}

func (p *StoragePool) SetAttributes(attributes map[string]sa.Offer) {
	p.attributes = attributes
}

func (p *StoragePool) InternalAttributes() map[string]string {
	return p.internalAttributes
}

func (p *StoragePool) SetInternalAttributes(internalAttributes map[string]string) {
	p.internalAttributes = internalAttributes
}

func (p *StoragePool) SupportedTopologies() []map[string]string {
	return p.supportedTopologies
}

func (p *StoragePool) SetSupportedTopologies(supportedTopologies []map[string]string) {
	p.supportedTopologies = supportedTopologies
}

func NewStoragePool(backend Backend, name string) *StoragePool {
	return &StoragePool{
		name:               name,
		storageClasses:     make([]string, 0),
		backend:            backend,
		attributes:         make(map[string]sa.Offer),
		internalAttributes: make(map[string]string),
	}
}

func (p *StoragePool) AddStorageClass(class string) {
	// Note that this function should get called once per storage class
	// affecting the volume; thus, we don't need to check for duplicates.
	p.storageClasses = append(p.storageClasses, class)
}

func (p *StoragePool) RemoveStorageClass(class string) bool {
	found := false
	for i, name := range p.storageClasses {
		if name == class {
			p.storageClasses = append(p.storageClasses[:i], p.storageClasses[i+1:]...)
			found = true
			break
		}
	}
	return found
}

type PoolExternal struct {
	Name           string   `json:"name"`
	StorageClasses []string `json:"storageClasses"`
	// TODO: can't have an interface here for unmarshalling
	Attributes          map[string]sa.Offer `json:"storageAttributes"`
	SupportedTopologies []map[string]string `json:"supportedTopologies"`
}

func (p *StoragePool) ConstructExternal() *PoolExternal {
	external := &PoolExternal{
		Name:                p.name,
		StorageClasses:      p.storageClasses,
		Attributes:          p.attributes,
		SupportedTopologies: p.supportedTopologies,
	}

	// We want to sort these so that the output remains consistent;
	// there are cases where the order won't always be the same.
	sort.Strings(external.StorageClasses)
	return external
}

// GetLabelsJSON returns a JSON-formatted string containing the labels on this pool, suitable
// for a label set on a storage volume.  The outer key may be customized.  For example:
// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}
func (p *StoragePool) GetLabelsJSON(ctx context.Context, key string, labelLimit int) (string, error) {
	labelOffer, ok := p.attributes[sa.Labels].(sa.LabelOffer)
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
			"maxLabelLength":   labelLimit,
		}).Error("label length exceeds the character limit")
		return "", fmt.Errorf("label length %v exceeds the character limit of %v characters", len(labelsJSON),
			labelLimit)
	}

	return labelsJSON, nil
}

// GetLabels returns a map containing the labels on this pool, suitable for individual
// metadata key/value pairs set on a storage volume.  Each key may be customized with
// a common prefix, unless it already has a prefix as detected by the presence of a slash.
// For example:
// {"prefix/cloud":"anf", "prefix/clusterName":"dev-test-cluster-1", "otherPrefix/tier":"hot"}
func (p *StoragePool) GetLabels(_ context.Context, prefix string) map[string]string {
	labelMap := make(map[string]string)

	labelOffer, ok := p.attributes[sa.Labels].(sa.LabelOffer)
	if !ok {
		return labelMap
	}

	for key, value := range labelOffer.Labels() {
		if strings.Contains(key, "/") {
			labelMap[key] = value
		} else {
			labelMap[prefix+key] = value
		}
	}

	return labelMap
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

func IsStoragePoolUnset(storagePool Pool) bool {
	return storagePool == nil || storagePool.Name() == drivers.UnsetPool
}
