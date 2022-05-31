// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeVmAlignAttributesType is a structure to represent a volume-vm-align-attributes ZAPI object
type VolumeVmAlignAttributesType struct {
	XMLName          xml.Name `xml:"volume-vm-align-attributes"`
	VmAlignSectorPtr *int     `xml:"vm-align-sector"`
	VmAlignSuffixPtr *string  `xml:"vm-align-suffix"`
}

// NewVolumeVmAlignAttributesType is a factory method for creating new instances of VolumeVmAlignAttributesType objects
func NewVolumeVmAlignAttributesType() *VolumeVmAlignAttributesType {
	return &VolumeVmAlignAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeVmAlignAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeVmAlignAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VmAlignSector is a 'getter' method
func (o *VolumeVmAlignAttributesType) VmAlignSector() int {
	var r int
	if o.VmAlignSectorPtr == nil {
		return r
	}
	r = *o.VmAlignSectorPtr
	return r
}

// SetVmAlignSector is a fluent style 'setter' method that can be chained
func (o *VolumeVmAlignAttributesType) SetVmAlignSector(newValue int) *VolumeVmAlignAttributesType {
	o.VmAlignSectorPtr = &newValue
	return o
}

// VmAlignSuffix is a 'getter' method
func (o *VolumeVmAlignAttributesType) VmAlignSuffix() string {
	var r string
	if o.VmAlignSuffixPtr == nil {
		return r
	}
	r = *o.VmAlignSuffixPtr
	return r
}

// SetVmAlignSuffix is a fluent style 'setter' method that can be chained
func (o *VolumeVmAlignAttributesType) SetVmAlignSuffix(newValue string) *VolumeVmAlignAttributesType {
	o.VmAlignSuffixPtr = &newValue
	return o
}
