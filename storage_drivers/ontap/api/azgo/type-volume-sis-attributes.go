// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeSisAttributesType is a structure to represent a volume-sis-attributes ZAPI object
type VolumeSisAttributesType struct {
	XMLName                              xml.Name  `xml:"volume-sis-attributes"`
	CompressionSpaceSavedPtr             *int      `xml:"compression-space-saved"`
	DeduplicationSpaceSavedPtr           *int      `xml:"deduplication-space-saved"`
	DeduplicationSpaceSharedPtr          *SizeType `xml:"deduplication-space-shared"`
	IsSisLoggingEnabledPtr               *bool     `xml:"is-sis-logging-enabled"`
	IsSisStateEnabledPtr                 *bool     `xml:"is-sis-state-enabled"`
	IsSisVolumePtr                       *bool     `xml:"is-sis-volume"`
	PercentageCompressionSpaceSavedPtr   *int      `xml:"percentage-compression-space-saved"`
	PercentageDeduplicationSpaceSavedPtr *int      `xml:"percentage-deduplication-space-saved"`
	PercentageTotalSpaceSavedPtr         *int      `xml:"percentage-total-space-saved"`
	TotalSpaceSavedPtr                   *int      `xml:"total-space-saved"`
}

// NewVolumeSisAttributesType is a factory method for creating new instances of VolumeSisAttributesType objects
func NewVolumeSisAttributesType() *VolumeSisAttributesType {
	return &VolumeSisAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSisAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSisAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CompressionSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) CompressionSpaceSaved() int {
	var r int
	if o.CompressionSpaceSavedPtr == nil {
		return r
	}
	r = *o.CompressionSpaceSavedPtr
	return r
}

// SetCompressionSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetCompressionSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.CompressionSpaceSavedPtr = &newValue
	return o
}

// DeduplicationSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) DeduplicationSpaceSaved() int {
	var r int
	if o.DeduplicationSpaceSavedPtr == nil {
		return r
	}
	r = *o.DeduplicationSpaceSavedPtr
	return r
}

// SetDeduplicationSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetDeduplicationSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.DeduplicationSpaceSavedPtr = &newValue
	return o
}

// DeduplicationSpaceShared is a 'getter' method
func (o *VolumeSisAttributesType) DeduplicationSpaceShared() SizeType {
	var r SizeType
	if o.DeduplicationSpaceSharedPtr == nil {
		return r
	}
	r = *o.DeduplicationSpaceSharedPtr
	return r
}

// SetDeduplicationSpaceShared is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetDeduplicationSpaceShared(newValue SizeType) *VolumeSisAttributesType {
	o.DeduplicationSpaceSharedPtr = &newValue
	return o
}

// IsSisLoggingEnabled is a 'getter' method
func (o *VolumeSisAttributesType) IsSisLoggingEnabled() bool {
	var r bool
	if o.IsSisLoggingEnabledPtr == nil {
		return r
	}
	r = *o.IsSisLoggingEnabledPtr
	return r
}

// SetIsSisLoggingEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetIsSisLoggingEnabled(newValue bool) *VolumeSisAttributesType {
	o.IsSisLoggingEnabledPtr = &newValue
	return o
}

// IsSisStateEnabled is a 'getter' method
func (o *VolumeSisAttributesType) IsSisStateEnabled() bool {
	var r bool
	if o.IsSisStateEnabledPtr == nil {
		return r
	}
	r = *o.IsSisStateEnabledPtr
	return r
}

// SetIsSisStateEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetIsSisStateEnabled(newValue bool) *VolumeSisAttributesType {
	o.IsSisStateEnabledPtr = &newValue
	return o
}

// IsSisVolume is a 'getter' method
func (o *VolumeSisAttributesType) IsSisVolume() bool {
	var r bool
	if o.IsSisVolumePtr == nil {
		return r
	}
	r = *o.IsSisVolumePtr
	return r
}

// SetIsSisVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetIsSisVolume(newValue bool) *VolumeSisAttributesType {
	o.IsSisVolumePtr = &newValue
	return o
}

// PercentageCompressionSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) PercentageCompressionSpaceSaved() int {
	var r int
	if o.PercentageCompressionSpaceSavedPtr == nil {
		return r
	}
	r = *o.PercentageCompressionSpaceSavedPtr
	return r
}

// SetPercentageCompressionSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetPercentageCompressionSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.PercentageCompressionSpaceSavedPtr = &newValue
	return o
}

// PercentageDeduplicationSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) PercentageDeduplicationSpaceSaved() int {
	var r int
	if o.PercentageDeduplicationSpaceSavedPtr == nil {
		return r
	}
	r = *o.PercentageDeduplicationSpaceSavedPtr
	return r
}

// SetPercentageDeduplicationSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetPercentageDeduplicationSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.PercentageDeduplicationSpaceSavedPtr = &newValue
	return o
}

// PercentageTotalSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) PercentageTotalSpaceSaved() int {
	var r int
	if o.PercentageTotalSpaceSavedPtr == nil {
		return r
	}
	r = *o.PercentageTotalSpaceSavedPtr
	return r
}

// SetPercentageTotalSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetPercentageTotalSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.PercentageTotalSpaceSavedPtr = &newValue
	return o
}

// TotalSpaceSaved is a 'getter' method
func (o *VolumeSisAttributesType) TotalSpaceSaved() int {
	var r int
	if o.TotalSpaceSavedPtr == nil {
		return r
	}
	r = *o.TotalSpaceSavedPtr
	return r
}

// SetTotalSpaceSaved is a fluent style 'setter' method that can be chained
func (o *VolumeSisAttributesType) SetTotalSpaceSaved(newValue int) *VolumeSisAttributesType {
	o.TotalSpaceSavedPtr = &newValue
	return o
}
