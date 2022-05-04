// Code generated automatically. DO NOT EDIT.
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
	r := *o.ExtentEnabledPtr
	return r
}

// SetExtentEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetExtentEnabled(newValue string) *VolumePerformanceAttributesType {
	o.ExtentEnabledPtr = &newValue
	return o
}

// FcDelegsEnabled is a 'getter' method
func (o *VolumePerformanceAttributesType) FcDelegsEnabled() bool {
	r := *o.FcDelegsEnabledPtr
	return r
}

// SetFcDelegsEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetFcDelegsEnabled(newValue bool) *VolumePerformanceAttributesType {
	o.FcDelegsEnabledPtr = &newValue
	return o
}

// IsAtimeUpdateEnabled is a 'getter' method
func (o *VolumePerformanceAttributesType) IsAtimeUpdateEnabled() bool {
	r := *o.IsAtimeUpdateEnabledPtr
	return r
}

// SetIsAtimeUpdateEnabled is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetIsAtimeUpdateEnabled(newValue bool) *VolumePerformanceAttributesType {
	o.IsAtimeUpdateEnabledPtr = &newValue
	return o
}

// MaxWriteAllocBlocks is a 'getter' method
func (o *VolumePerformanceAttributesType) MaxWriteAllocBlocks() int {
	r := *o.MaxWriteAllocBlocksPtr
	return r
}

// SetMaxWriteAllocBlocks is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetMaxWriteAllocBlocks(newValue int) *VolumePerformanceAttributesType {
	o.MaxWriteAllocBlocksPtr = &newValue
	return o
}

// MinimalReadAhead is a 'getter' method
func (o *VolumePerformanceAttributesType) MinimalReadAhead() bool {
	r := *o.MinimalReadAheadPtr
	return r
}

// SetMinimalReadAhead is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetMinimalReadAhead(newValue bool) *VolumePerformanceAttributesType {
	o.MinimalReadAheadPtr = &newValue
	return o
}

// ReadRealloc is a 'getter' method
func (o *VolumePerformanceAttributesType) ReadRealloc() string {
	r := *o.ReadReallocPtr
	return r
}

// SetReadRealloc is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetReadRealloc(newValue string) *VolumePerformanceAttributesType {
	o.ReadReallocPtr = &newValue
	return o
}

// SingleInstanceDataLogging is a 'getter' method
func (o *VolumePerformanceAttributesType) SingleInstanceDataLogging() string {
	r := *o.SingleInstanceDataLoggingPtr
	return r
}

// SetSingleInstanceDataLogging is a fluent style 'setter' method that can be chained
func (o *VolumePerformanceAttributesType) SetSingleInstanceDataLogging(newValue string) *VolumePerformanceAttributesType {
	o.SingleInstanceDataLoggingPtr = &newValue
	return o
}
