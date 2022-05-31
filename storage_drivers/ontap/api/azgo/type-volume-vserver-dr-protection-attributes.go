// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeVserverDrProtectionAttributesType is a structure to represent a volume-vserver-dr-protection-attributes ZAPI object
type VolumeVserverDrProtectionAttributesType struct {
	XMLName                xml.Name `xml:"volume-vserver-dr-protection-attributes"`
	VserverDrProtectionPtr *string  `xml:"vserver-dr-protection"`
}

// NewVolumeVserverDrProtectionAttributesType is a factory method for creating new instances of VolumeVserverDrProtectionAttributesType objects
func NewVolumeVserverDrProtectionAttributesType() *VolumeVserverDrProtectionAttributesType {
	return &VolumeVserverDrProtectionAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeVserverDrProtectionAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeVserverDrProtectionAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverDrProtection is a 'getter' method
func (o *VolumeVserverDrProtectionAttributesType) VserverDrProtection() string {
	var r string
	if o.VserverDrProtectionPtr == nil {
		return r
	}
	r = *o.VserverDrProtectionPtr
	return r
}

// SetVserverDrProtection is a fluent style 'setter' method that can be chained
func (o *VolumeVserverDrProtectionAttributesType) SetVserverDrProtection(newValue string) *VolumeVserverDrProtectionAttributesType {
	o.VserverDrProtectionPtr = &newValue
	return o
}
