// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeHybridCacheAttributesType is a structure to represent a volume-hybrid-cache-attributes ZAPI object
type VolumeHybridCacheAttributesType struct {
	XMLName                          xml.Name `xml:"volume-hybrid-cache-attributes"`
	CacheRetentionPriorityPtr        *string  `xml:"cache-retention-priority"`
	CachingPolicyPtr                 *string  `xml:"caching-policy"`
	EligibilityPtr                   *string  `xml:"eligibility"`
	WriteCacheIneligibilityReasonPtr *string  `xml:"write-cache-ineligibility-reason"`
}

// NewVolumeHybridCacheAttributesType is a factory method for creating new instances of VolumeHybridCacheAttributesType objects
func NewVolumeHybridCacheAttributesType() *VolumeHybridCacheAttributesType {
	return &VolumeHybridCacheAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeHybridCacheAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeHybridCacheAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CacheRetentionPriority is a 'getter' method
func (o *VolumeHybridCacheAttributesType) CacheRetentionPriority() string {
	var r string
	if o.CacheRetentionPriorityPtr == nil {
		return r
	}
	r = *o.CacheRetentionPriorityPtr
	return r
}

// SetCacheRetentionPriority is a fluent style 'setter' method that can be chained
func (o *VolumeHybridCacheAttributesType) SetCacheRetentionPriority(newValue string) *VolumeHybridCacheAttributesType {
	o.CacheRetentionPriorityPtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *VolumeHybridCacheAttributesType) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeHybridCacheAttributesType) SetCachingPolicy(newValue string) *VolumeHybridCacheAttributesType {
	o.CachingPolicyPtr = &newValue
	return o
}

// Eligibility is a 'getter' method
func (o *VolumeHybridCacheAttributesType) Eligibility() string {
	var r string
	if o.EligibilityPtr == nil {
		return r
	}
	r = *o.EligibilityPtr
	return r
}

// SetEligibility is a fluent style 'setter' method that can be chained
func (o *VolumeHybridCacheAttributesType) SetEligibility(newValue string) *VolumeHybridCacheAttributesType {
	o.EligibilityPtr = &newValue
	return o
}

// WriteCacheIneligibilityReason is a 'getter' method
func (o *VolumeHybridCacheAttributesType) WriteCacheIneligibilityReason() string {
	var r string
	if o.WriteCacheIneligibilityReasonPtr == nil {
		return r
	}
	r = *o.WriteCacheIneligibilityReasonPtr
	return r
}

// SetWriteCacheIneligibilityReason is a fluent style 'setter' method that can be chained
func (o *VolumeHybridCacheAttributesType) SetWriteCacheIneligibilityReason(newValue string) *VolumeHybridCacheAttributesType {
	o.WriteCacheIneligibilityReasonPtr = &newValue
	return o
}
