// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumePerformanceAttributesType is a structure to represent a volume-performance-attributes ZAPI object
type VolumePerformanceAttributesType struct {
	XMLName                      xml.Name `xml:"volume-performance-attributes"`
	ExtentEnabledPtr             *string  `xml:"extent-enabled"`
	FcDelegsEnabledPtr           *bool    `xml:"fc-delegs-enabled"`
	IsAtimeUpdateEnabledPtr      *bool    `xml:"is-atime-update-enabled"`
	MaxWriteAllocBlocksPtr       *int     `xml:"max-write-alloc-blocks"`
	MinimalReadAheadPtr          *bool    `xml:"minimal-read-ahead"`
	ReadReallocPtr               *string  `xml:"read-realloc"`
	SingleInstanceDataLoggingPtr *string  `xml:"single-instance-data-logging"`
}

// NewVolumePerformanceAttributesType is a factory method for creating new instances of VolumePerformanceAttributesType objects
func NewVolumePerformanceAttributesType() *VolumePerformanceAttributesType {
	return &VolumePerformanceAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumePerformanceAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumePerformanceAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExtentEnabled is a 'getter' method
func (o *VolumePerformanceAttributesType) ExtentEnabled() string {
	var r string
	if o.ExtentEnabledPtr == nil {
		return r
	}
	r = *o.ExtentEnabledPtr
	return r
}

// SetExtentEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetExtentEnabled(newValue string) *VolumePerformanceAttributesType {
	o.ExtentEnabledPtr = &newValue
	return o
}

// FcDelegsEnabled is a 'getter' method
func (o *VolumePerformanceAttributesType) FcDelegsEnabled() bool {
	var r bool
	if o.FcDelegsEnabledPtr == nil {
		return r
	}
	r = *o.FcDelegsEnabledPtr
	return r
}

// SetFcDelegsEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetFcDelegsEnabled(newValue bool) *VolumePerformanceAttributesType {
	o.FcDelegsEnabledPtr = &newValue
	return o
}

// IsAtimeUpdateEnabled is a 'getter' method
func (o *VolumePerformanceAttributesType) IsAtimeUpdateEnabled() bool {
	var r bool
	if o.IsAtimeUpdateEnabledPtr == nil {
		return r
	}
	r = *o.IsAtimeUpdateEnabledPtr
	return r
}

// SetIsAtimeUpdateEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetIsAtimeUpdateEnabled(newValue bool) *VolumePerformanceAttributesType {
	o.IsAtimeUpdateEnabledPtr = &newValue
	return o
}

// MaxWriteAllocBlocks is a 'getter' method
func (o *VolumePerformanceAttributesType) MaxWriteAllocBlocks() int {
	var r int
	if o.MaxWriteAllocBlocksPtr == nil {
		return r
	}
	r = *o.MaxWriteAllocBlocksPtr
	return r
}

// SetMaxWriteAllocBlocks is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetMaxWriteAllocBlocks(newValue int) *VolumePerformanceAttributesType {
	o.MaxWriteAllocBlocksPtr = &newValue
	return o
}

// MinimalReadAhead is a 'getter' method
func (o *VolumePerformanceAttributesType) MinimalReadAhead() bool {
	var r bool
	if o.MinimalReadAheadPtr == nil {
		return r
	}
	r = *o.MinimalReadAheadPtr
	return r
}

// SetMinimalReadAhead is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetMinimalReadAhead(newValue bool) *VolumePerformanceAttributesType {
	o.MinimalReadAheadPtr = &newValue
	return o
}

// ReadRealloc is a 'getter' method
func (o *VolumePerformanceAttributesType) ReadRealloc() string {
	var r string
	if o.ReadReallocPtr == nil {
		return r
	}
	r = *o.ReadReallocPtr
	return r
}

// SetReadRealloc is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetReadRealloc(newValue string) *VolumePerformanceAttributesType {
	o.ReadReallocPtr = &newValue
	return o
}

// SingleInstanceDataLogging is a 'getter' method
func (o *VolumePerformanceAttributesType) SingleInstanceDataLogging() string {
	var r string
	if o.SingleInstanceDataLoggingPtr == nil {
		return r
	}
	r = *o.SingleInstanceDataLoggingPtr
	return r
}

// SetSingleInstanceDataLogging is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetSingleInstanceDataLogging(newValue string) *VolumePerformanceAttributesType {
	o.SingleInstanceDataLoggingPtr = &newValue
	return o
}
