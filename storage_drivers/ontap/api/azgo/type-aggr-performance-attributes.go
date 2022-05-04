// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrPerformanceAttributesType is a structure to represent a aggr-performance-attributes ZAPI object
type AggrPerformanceAttributesType struct {
	XMLName                      xml.Name `xml:"aggr-performance-attributes"`
	FreeSpaceReallocPtr          *string  `xml:"free-space-realloc"`
	MaxWriteAllocBlocksPtr       *int     `xml:"max-write-alloc-blocks"`
	SingleInstanceDataLoggingPtr *string  `xml:"single-instance-data-logging"`
}

// NewAggrPerformanceAttributesType is a factory method for creating new instances of AggrPerformanceAttributesType objects
func NewAggrPerformanceAttributesType() *AggrPerformanceAttributesType {
	return &AggrPerformanceAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrPerformanceAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrPerformanceAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// FreeSpaceRealloc is a 'getter' method
func (o *AggrPerformanceAttributesType) FreeSpaceRealloc() string {
	r := *o.FreeSpaceReallocPtr
	return r
}

// SetFreeSpaceRealloc is a fluent style 'setter' method that can be chained
func (o *AggrPerformanceAttributesType) SetFreeSpaceRealloc(newValue string) *AggrPerformanceAttributesType {
	o.FreeSpaceReallocPtr = &newValue
	return o
}

// MaxWriteAllocBlocks is a 'getter' method
func (o *AggrPerformanceAttributesType) MaxWriteAllocBlocks() int {
	r := *o.MaxWriteAllocBlocksPtr
	return r
}

// SetMaxWriteAllocBlocks is a fluent style 'setter' method that can be chained
func (o *AggrPerformanceAttributesType) SetMaxWriteAllocBlocks(newValue int) *AggrPerformanceAttributesType {
	o.MaxWriteAllocBlocksPtr = &newValue
	return o
}

// SingleInstanceDataLogging is a 'getter' method
func (o *AggrPerformanceAttributesType) SingleInstanceDataLogging() string {
	r := *o.SingleInstanceDataLoggingPtr
	return r
}

// SetSingleInstanceDataLogging is a fluent style 'setter' method that can be chained
func (o *AggrPerformanceAttributesType) SetSingleInstanceDataLogging(newValue string) *AggrPerformanceAttributesType {
	o.SingleInstanceDataLoggingPtr = &newValue
	return o
}
