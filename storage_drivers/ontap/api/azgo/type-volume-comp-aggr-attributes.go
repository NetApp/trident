// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeCompAggrAttributesType is a structure to represent a volume-comp-aggr-attributes ZAPI object
type VolumeCompAggrAttributesType struct {
	XMLName                      xml.Name                                       `xml:"volume-comp-aggr-attributes"`
	CloudRetrievalPolicyPtr      *string                                        `xml:"cloud-retrieval-policy"`
	NeedsObjectRetaggingPtr      *bool                                          `xml:"needs-object-retagging"`
	TieringMinimumCoolingDaysPtr *int                                           `xml:"tiering-minimum-cooling-days"`
	TieringObjectTagsPtr         *VolumeCompAggrAttributesTypeTieringObjectTags `xml:"tiering-object-tags"`
	// work in progress
	TieringPolicyPtr *string `xml:"tiering-policy"`
}

// NewVolumeCompAggrAttributesType is a factory method for creating new instances of VolumeCompAggrAttributesType objects
func NewVolumeCompAggrAttributesType() *VolumeCompAggrAttributesType {
	return &VolumeCompAggrAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCompAggrAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCompAggrAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CloudRetrievalPolicy is a 'getter' method
func (o *VolumeCompAggrAttributesType) CloudRetrievalPolicy() string {
	var r string
	if o.CloudRetrievalPolicyPtr == nil {
		return r
	}
	r = *o.CloudRetrievalPolicyPtr
	return r
}

// SetCloudRetrievalPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetCloudRetrievalPolicy(newValue string) *VolumeCompAggrAttributesType {
	o.CloudRetrievalPolicyPtr = &newValue
	return o
}

// NeedsObjectRetagging is a 'getter' method
func (o *VolumeCompAggrAttributesType) NeedsObjectRetagging() bool {
	var r bool
	if o.NeedsObjectRetaggingPtr == nil {
		return r
	}
	r = *o.NeedsObjectRetaggingPtr
	return r
}

// SetNeedsObjectRetagging is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetNeedsObjectRetagging(newValue bool) *VolumeCompAggrAttributesType {
	o.NeedsObjectRetaggingPtr = &newValue
	return o
}

// TieringMinimumCoolingDays is a 'getter' method
func (o *VolumeCompAggrAttributesType) TieringMinimumCoolingDays() int {
	var r int
	if o.TieringMinimumCoolingDaysPtr == nil {
		return r
	}
	r = *o.TieringMinimumCoolingDaysPtr
	return r
}

// SetTieringMinimumCoolingDays is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetTieringMinimumCoolingDays(newValue int) *VolumeCompAggrAttributesType {
	o.TieringMinimumCoolingDaysPtr = &newValue
	return o
}

// VolumeCompAggrAttributesTypeTieringObjectTags is a wrapper
type VolumeCompAggrAttributesTypeTieringObjectTags struct {
	XMLName   xml.Name `xml:"tiering-object-tags"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *VolumeCompAggrAttributesTypeTieringObjectTags) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesTypeTieringObjectTags) SetString(newValue []string) *VolumeCompAggrAttributesTypeTieringObjectTags {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// TieringObjectTags is a 'getter' method
func (o *VolumeCompAggrAttributesType) TieringObjectTags() VolumeCompAggrAttributesTypeTieringObjectTags {
	var r VolumeCompAggrAttributesTypeTieringObjectTags
	if o.TieringObjectTagsPtr == nil {
		return r
	}
	r = *o.TieringObjectTagsPtr
	return r
}

// SetTieringObjectTags is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetTieringObjectTags(newValue VolumeCompAggrAttributesTypeTieringObjectTags) *VolumeCompAggrAttributesType {
	o.TieringObjectTagsPtr = &newValue
	return o
}

// TieringPolicy is a 'getter' method
func (o *VolumeCompAggrAttributesType) TieringPolicy() string {
	var r string
	if o.TieringPolicyPtr == nil {
		return r
	}
	r = *o.TieringPolicyPtr
	return r
}

// SetTieringPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetTieringPolicy(newValue string) *VolumeCompAggrAttributesType {
	o.TieringPolicyPtr = &newValue
	return o
}
