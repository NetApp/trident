// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VmSystemDisksType is a structure to represent a vm-system-disks ZAPI object
type VmSystemDisksType struct {
	XMLName               xml.Name `xml:"vm-system-disks"`
	VmBootdiskAreaNamePtr *string  `xml:"vm-bootdisk-area-name"`
	VmBootdiskFileNamePtr *string  `xml:"vm-bootdisk-file-name"`
	VmCorediskAreaNamePtr *string  `xml:"vm-coredisk-area-name"`
	VmCorediskFileNamePtr *string  `xml:"vm-coredisk-file-name"`
	VmLogdiskAreaNamePtr  *string  `xml:"vm-logdisk-area-name"`
	VmLogdiskFileNamePtr  *string  `xml:"vm-logdisk-file-name"`
}

// NewVmSystemDisksType is a factory method for creating new instances of VmSystemDisksType objects
func NewVmSystemDisksType() *VmSystemDisksType {
	return &VmSystemDisksType{}
}

// ToXML converts this object into an xml string representation
func (o *VmSystemDisksType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VmSystemDisksType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VmBootdiskAreaName is a 'getter' method
func (o *VmSystemDisksType) VmBootdiskAreaName() string {
	var r string
	if o.VmBootdiskAreaNamePtr == nil {
		return r
	}
	r = *o.VmBootdiskAreaNamePtr
	return r
}

// SetVmBootdiskAreaName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmBootdiskAreaName(newValue string) *VmSystemDisksType {
	o.VmBootdiskAreaNamePtr = &newValue
	return o
}

// VmBootdiskFileName is a 'getter' method
func (o *VmSystemDisksType) VmBootdiskFileName() string {
	var r string
	if o.VmBootdiskFileNamePtr == nil {
		return r
	}
	r = *o.VmBootdiskFileNamePtr
	return r
}

// SetVmBootdiskFileName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmBootdiskFileName(newValue string) *VmSystemDisksType {
	o.VmBootdiskFileNamePtr = &newValue
	return o
}

// VmCorediskAreaName is a 'getter' method
func (o *VmSystemDisksType) VmCorediskAreaName() string {
	var r string
	if o.VmCorediskAreaNamePtr == nil {
		return r
	}
	r = *o.VmCorediskAreaNamePtr
	return r
}

// SetVmCorediskAreaName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmCorediskAreaName(newValue string) *VmSystemDisksType {
	o.VmCorediskAreaNamePtr = &newValue
	return o
}

// VmCorediskFileName is a 'getter' method
func (o *VmSystemDisksType) VmCorediskFileName() string {
	var r string
	if o.VmCorediskFileNamePtr == nil {
		return r
	}
	r = *o.VmCorediskFileNamePtr
	return r
}

// SetVmCorediskFileName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmCorediskFileName(newValue string) *VmSystemDisksType {
	o.VmCorediskFileNamePtr = &newValue
	return o
}

// VmLogdiskAreaName is a 'getter' method
func (o *VmSystemDisksType) VmLogdiskAreaName() string {
	var r string
	if o.VmLogdiskAreaNamePtr == nil {
		return r
	}
	r = *o.VmLogdiskAreaNamePtr
	return r
}

// SetVmLogdiskAreaName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmLogdiskAreaName(newValue string) *VmSystemDisksType {
	o.VmLogdiskAreaNamePtr = &newValue
	return o
}

// VmLogdiskFileName is a 'getter' method
func (o *VmSystemDisksType) VmLogdiskFileName() string {
	var r string
	if o.VmLogdiskFileNamePtr == nil {
		return r
	}
	r = *o.VmLogdiskFileNamePtr
	return r
}

// SetVmLogdiskFileName is a fluent style 'setter' method that can be chained
func (o *VmSystemDisksType) SetVmLogdiskFileName(newValue string) *VmSystemDisksType {
	o.VmLogdiskFileNamePtr = &newValue
	return o
}
