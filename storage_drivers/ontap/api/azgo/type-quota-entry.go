// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// QuotaEntryType is a structure to represent a quota-entry ZAPI object
type QuotaEntryType struct {
	XMLName               xml.Name `xml:"quota-entry"`
	DiskLimitPtr          *string  `xml:"disk-limit"`
	FileLimitPtr          *string  `xml:"file-limit"`
	PerformUserMappingPtr *bool    `xml:"perform-user-mapping"`
	PolicyPtr             *string  `xml:"policy"`
	QtreePtr              *string  `xml:"qtree"`
	QuotaTargetPtr        *string  `xml:"quota-target"`
	QuotaTypePtr          *string  `xml:"quota-type"`
	SoftDiskLimitPtr      *string  `xml:"soft-disk-limit"`
	SoftFileLimitPtr      *string  `xml:"soft-file-limit"`
	ThresholdPtr          *string  `xml:"threshold"`
	VolumePtr             *string  `xml:"volume"`
	VserverPtr            *string  `xml:"vserver"`
}

// NewQuotaEntryType is a factory method for creating new instances of QuotaEntryType objects
func NewQuotaEntryType() *QuotaEntryType {
	return &QuotaEntryType{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaEntryType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaEntryType) String() string {
	return ToString(reflect.ValueOf(o))
}

// DiskLimit is a 'getter' method
func (o *QuotaEntryType) DiskLimit() string {
	var r string
	if o.DiskLimitPtr == nil {
		return r
	}
	r = *o.DiskLimitPtr
	return r
}

// SetDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetDiskLimit(newValue string) *QuotaEntryType {
	o.DiskLimitPtr = &newValue
	return o
}

// FileLimit is a 'getter' method
func (o *QuotaEntryType) FileLimit() string {
	var r string
	if o.FileLimitPtr == nil {
		return r
	}
	r = *o.FileLimitPtr
	return r
}

// SetFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetFileLimit(newValue string) *QuotaEntryType {
	o.FileLimitPtr = &newValue
	return o
}

// PerformUserMapping is a 'getter' method
func (o *QuotaEntryType) PerformUserMapping() bool {
	var r bool
	if o.PerformUserMappingPtr == nil {
		return r
	}
	r = *o.PerformUserMappingPtr
	return r
}

// SetPerformUserMapping is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetPerformUserMapping(newValue bool) *QuotaEntryType {
	o.PerformUserMappingPtr = &newValue
	return o
}

// Policy is a 'getter' method
func (o *QuotaEntryType) Policy() string {
	var r string
	if o.PolicyPtr == nil {
		return r
	}
	r = *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetPolicy(newValue string) *QuotaEntryType {
	o.PolicyPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QuotaEntryType) Qtree() string {
	var r string
	if o.QtreePtr == nil {
		return r
	}
	r = *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetQtree(newValue string) *QuotaEntryType {
	o.QtreePtr = &newValue
	return o
}

// QuotaTarget is a 'getter' method
func (o *QuotaEntryType) QuotaTarget() string {
	var r string
	if o.QuotaTargetPtr == nil {
		return r
	}
	r = *o.QuotaTargetPtr
	return r
}

// SetQuotaTarget is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetQuotaTarget(newValue string) *QuotaEntryType {
	o.QuotaTargetPtr = &newValue
	return o
}

// QuotaType is a 'getter' method
func (o *QuotaEntryType) QuotaType() string {
	var r string
	if o.QuotaTypePtr == nil {
		return r
	}
	r = *o.QuotaTypePtr
	return r
}

// SetQuotaType is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetQuotaType(newValue string) *QuotaEntryType {
	o.QuotaTypePtr = &newValue
	return o
}

// SoftDiskLimit is a 'getter' method
func (o *QuotaEntryType) SoftDiskLimit() string {
	var r string
	if o.SoftDiskLimitPtr == nil {
		return r
	}
	r = *o.SoftDiskLimitPtr
	return r
}

// SetSoftDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetSoftDiskLimit(newValue string) *QuotaEntryType {
	o.SoftDiskLimitPtr = &newValue
	return o
}

// SoftFileLimit is a 'getter' method
func (o *QuotaEntryType) SoftFileLimit() string {
	var r string
	if o.SoftFileLimitPtr == nil {
		return r
	}
	r = *o.SoftFileLimitPtr
	return r
}

// SetSoftFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetSoftFileLimit(newValue string) *QuotaEntryType {
	o.SoftFileLimitPtr = &newValue
	return o
}

// Threshold is a 'getter' method
func (o *QuotaEntryType) Threshold() string {
	var r string
	if o.ThresholdPtr == nil {
		return r
	}
	r = *o.ThresholdPtr
	return r
}

// SetThreshold is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetThreshold(newValue string) *QuotaEntryType {
	o.ThresholdPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *QuotaEntryType) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetVolume(newValue string) *QuotaEntryType {
	o.VolumePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *QuotaEntryType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *QuotaEntryType) SetVserver(newValue string) *QuotaEntryType {
	o.VserverPtr = &newValue
	return o
}
