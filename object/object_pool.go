// Copyright 2026 NetApp, Inc. All Rights Reserved.

package object

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
)

const ProvisioningLabelTag = "provisioning"

type ObjectPool struct {
	name string
	// A Trident object pool can potentially satisfy more than one bucket class.
	bucketClasses      []string
	store              Store
	attributes         map[string]sa.Offer // These attributes are used to match bucket classes
	internalAttributes map[string]string   // These attributes are defined & used internally by object store drivers
}

func (p *ObjectPool) Name() string {
	return p.name
}

func (p *ObjectPool) SetName(name string) {
	p.name = name
}

func (p *ObjectPool) BucketClasses() []string {
	return p.bucketClasses
}

func (p *ObjectPool) SetBucketClasses(bucketClasses []string) {
	p.bucketClasses = bucketClasses
}

func (p *ObjectPool) Store() Store {
	return p.store
}

func (p *ObjectPool) SetStore(store Store) {
	p.store = store
}

func (p *ObjectPool) Attributes() map[string]sa.Offer {
	return p.attributes
}

func (p *ObjectPool) SetAttributes(attributes map[string]sa.Offer) {
	p.attributes = attributes
}

func (p *ObjectPool) InternalAttributes() map[string]string {
	return p.internalAttributes
}

func (p *ObjectPool) SetInternalAttributes(internalAttributes map[string]string) {
	p.internalAttributes = internalAttributes
}

func NewObjectPool(store Store, name string) *ObjectPool {
	return &ObjectPool{
		name:               name,
		bucketClasses:      make([]string, 0),
		store:              store,
		attributes:         make(map[string]sa.Offer),
		internalAttributes: make(map[string]string),
	}
}

func (p *ObjectPool) AddBucketClass(class string) {
	// Note that this function should get called once per bucket class
	// affecting the bucket; thus, we don't need to check for duplicates.
	p.bucketClasses = append(p.bucketClasses, class)
}

func (p *ObjectPool) RemoveBucketClass(class string) bool {
	found := false
	for i, name := range p.bucketClasses {
		if name == class {
			p.bucketClasses = append(p.bucketClasses[:i], p.bucketClasses[i+1:]...)
			found = true
			break
		}
	}
	return found
}

// GetLabelsJSON returns a JSON-formatted string containing the labels on this pool, suitable
// for a label set on a storage volume.  The outer key may be customized.  For example:
// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}
func (p *ObjectPool) GetLabelsJSON(ctx context.Context, key string, labelLimit int) (string, error) {
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
		Logc(ctx).WithFields(LogFields{
			"labelsJSON":       labelsJSON,
			"labelsJSONLength": len(labelsJSON),
			"maxLabelLength":   labelLimit,
		}).Error("label length exceeds the character limit")
		return "", fmt.Errorf("label length %v exceeds the character limit of %v characters", len(labelsJSON),
			labelLimit)
	}

	return labelsJSON, nil
}

// GetTemplatizedLabelsJSON returns a JSON-formatted string containing the labels on this pool, suitable
// for a label set on a storage volume.  The outer key may be customized.  For example:
// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}.
func (p *ObjectPool) GetTemplatizedLabelsJSON(
	ctx context.Context, key string, labelLimit int, templateData map[string]interface{},
) (string, error) {
	labelOffer, ok := p.attributes[sa.Labels].(sa.LabelOffer)
	if !ok {
		return "", nil
	}

	labelOfferMap := labelOffer.Labels()
	if len(labelOfferMap) == 0 {
		return "", nil
	}

	labelOfferMap = p.GetLabelMapFromTemplate(ctx, templateData)

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
		Logc(ctx).WithFields(LogFields{
			"labelsJSON":       labelsJSON,
			"labelsJSONLength": len(labelsJSON),
			"maxLabelLength":   labelLimit,
		}).Error("label length exceeds the character limit")
		return "", fmt.Errorf("label length %v exceeds the character limit of %v characters", len(labelsJSON),
			labelLimit)
	}

	return labelsJSON, nil
}

// GetLabelMapFromTemplate generates labels from a template. The method retrieves the current labels of the object pool
// and iterates over them. If a label value contains template placeholders (i.e., "{{" and "}}"), it attempts to parse
// and execute the template using the provided template data. The method returns a map of the generated labels.
func (p *ObjectPool) GetLabelMapFromTemplate(ctx context.Context, templateData map[string]interface{}) map[string]string {
	labelMap := p.GetLabels(ctx, "")

	for k, v := range labelMap {
		if !(strings.Contains(v, "{{") && strings.Contains(v, "}}")) {
			continue
		}
		tmpl, err := template.New("templatizedLabel").Parse(v)
		if err != nil {
			Logc(ctx).Errorf("Failed to parse label template %s:%s; %v", k, v, err)
			continue
		}

		var tBuffer bytes.Buffer
		err = tmpl.Execute(&tBuffer, templateData)
		if err != nil {
			Logc(ctx).Errorf("Failed to execute label template %s:%s; %v", k, v, err)
			continue
		}

		labelMap[k] = tBuffer.String()
	}
	return labelMap
}

// GetLabels returns a map containing the labels on this pool, suitable for individual
// metadata key/value pairs set on a storage volume.  Each key may be customized with
// a common prefix, unless it already has a prefix as detected by the presence of a slash.
// For example:
// {"prefix/cloud":"anf", "prefix/clusterName":"dev-test-cluster-1", "otherPrefix/tier":"hot"}
func (p *ObjectPool) GetLabels(_ context.Context, prefix string) map[string]string {
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

// AllowPoolLabelOverwrite returns true if it has a key we could have set. For example:
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

// UpdateProvisioningLabels returns the volume labels with an updated provisioning label provided
// Note:- Currently not used. Will be used for update storagevolume labels
func UpdateProvisioningLabels(provisioningLabel string, volumeLabels []string) []string {
	newLabels := DeleteProvisioningLabels(volumeLabels)
	return append(newLabels, provisioningLabel)
}

// DeleteProvisioningLabels returns the volume labels with the provisioning label deleted
func DeleteProvisioningLabels(volumeLabels []string) []string {
	newLabels := make([]string, 0)

	for _, label := range volumeLabels {
		if !AllowPoolLabelOverwrite(ProvisioningLabelTag, label) {
			newLabels = append(newLabels, label)
		}
	}

	return newLabels
}
