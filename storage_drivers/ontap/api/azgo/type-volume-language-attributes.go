// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeLanguageAttributesType is a structure to represent a volume-language-attributes ZAPI object
type VolumeLanguageAttributesType struct {
	XMLName                  xml.Name          `xml:"volume-language-attributes"`
	IsConvertUcodeEnabledPtr *bool             `xml:"is-convert-ucode-enabled"`
	IsCreateUcodeEnabledPtr  *bool             `xml:"is-create-ucode-enabled"`
	LanguagePtr              *string           `xml:"language"`
	LanguageCodePtr          *LanguageCodeType `xml:"language-code"`
	NfsCharacterSetPtr       *string           `xml:"nfs-character-set"`
	OemCharacterSetPtr       *string           `xml:"oem-character-set"`
}

// NewVolumeLanguageAttributesType is a factory method for creating new instances of VolumeLanguageAttributesType objects
func NewVolumeLanguageAttributesType() *VolumeLanguageAttributesType {
	return &VolumeLanguageAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeLanguageAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeLanguageAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsConvertUcodeEnabled is a 'getter' method
func (o *VolumeLanguageAttributesType) IsConvertUcodeEnabled() bool {
	var r bool
	if o.IsConvertUcodeEnabledPtr == nil {
		return r
	}
	r = *o.IsConvertUcodeEnabledPtr
	return r
}

// SetIsConvertUcodeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetIsConvertUcodeEnabled(newValue bool) *VolumeLanguageAttributesType {
	o.IsConvertUcodeEnabledPtr = &newValue
	return o
}

// IsCreateUcodeEnabled is a 'getter' method
func (o *VolumeLanguageAttributesType) IsCreateUcodeEnabled() bool {
	var r bool
	if o.IsCreateUcodeEnabledPtr == nil {
		return r
	}
	r = *o.IsCreateUcodeEnabledPtr
	return r
}

// SetIsCreateUcodeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetIsCreateUcodeEnabled(newValue bool) *VolumeLanguageAttributesType {
	o.IsCreateUcodeEnabledPtr = &newValue
	return o
}

// Language is a 'getter' method
func (o *VolumeLanguageAttributesType) Language() string {
	var r string
	if o.LanguagePtr == nil {
		return r
	}
	r = *o.LanguagePtr
	return r
}

// SetLanguage is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetLanguage(newValue string) *VolumeLanguageAttributesType {
	o.LanguagePtr = &newValue
	return o
}

// LanguageCode is a 'getter' method
func (o *VolumeLanguageAttributesType) LanguageCode() LanguageCodeType {
	var r LanguageCodeType
	if o.LanguageCodePtr == nil {
		return r
	}
	r = *o.LanguageCodePtr
	return r
}

// SetLanguageCode is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetLanguageCode(newValue LanguageCodeType) *VolumeLanguageAttributesType {
	o.LanguageCodePtr = &newValue
	return o
}

// NfsCharacterSet is a 'getter' method
func (o *VolumeLanguageAttributesType) NfsCharacterSet() string {
	var r string
	if o.NfsCharacterSetPtr == nil {
		return r
	}
	r = *o.NfsCharacterSetPtr
	return r
}

// SetNfsCharacterSet is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetNfsCharacterSet(newValue string) *VolumeLanguageAttributesType {
	o.NfsCharacterSetPtr = &newValue
	return o
}

// OemCharacterSet is a 'getter' method
func (o *VolumeLanguageAttributesType) OemCharacterSet() string {
	var r string
	if o.OemCharacterSetPtr == nil {
		return r
	}
	r = *o.OemCharacterSetPtr
	return r
}

// SetOemCharacterSet is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetOemCharacterSet(newValue string) *VolumeLanguageAttributesType {
	o.OemCharacterSetPtr = &newValue
	return o
}
