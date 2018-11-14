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
	r := *o.IsConvertUcodeEnabledPtr
	return r
}

// SetIsConvertUcodeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetIsConvertUcodeEnabled(newValue bool) *VolumeLanguageAttributesType {
	o.IsConvertUcodeEnabledPtr = &newValue
	return o
}

// IsCreateUcodeEnabled is a 'getter' method
func (o *VolumeLanguageAttributesType) IsCreateUcodeEnabled() bool {
	r := *o.IsCreateUcodeEnabledPtr
	return r
}

// SetIsCreateUcodeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetIsCreateUcodeEnabled(newValue bool) *VolumeLanguageAttributesType {
	o.IsCreateUcodeEnabledPtr = &newValue
	return o
}

// Language is a 'getter' method
func (o *VolumeLanguageAttributesType) Language() string {
	r := *o.LanguagePtr
	return r
}

// SetLanguage is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetLanguage(newValue string) *VolumeLanguageAttributesType {
	o.LanguagePtr = &newValue
	return o
}

// LanguageCode is a 'getter' method
func (o *VolumeLanguageAttributesType) LanguageCode() LanguageCodeType {
	r := *o.LanguageCodePtr
	return r
}

// SetLanguageCode is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetLanguageCode(newValue LanguageCodeType) *VolumeLanguageAttributesType {
	o.LanguageCodePtr = &newValue
	return o
}

// NfsCharacterSet is a 'getter' method
func (o *VolumeLanguageAttributesType) NfsCharacterSet() string {
	r := *o.NfsCharacterSetPtr
	return r
}

// SetNfsCharacterSet is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetNfsCharacterSet(newValue string) *VolumeLanguageAttributesType {
	o.NfsCharacterSetPtr = &newValue
	return o
}

// OemCharacterSet is a 'getter' method
func (o *VolumeLanguageAttributesType) OemCharacterSet() string {
	r := *o.OemCharacterSetPtr
	return r
}

// SetOemCharacterSet is a fluent style 'setter' method that can be chained
func (o *VolumeLanguageAttributesType) SetOemCharacterSet(newValue string) *VolumeLanguageAttributesType {
	o.OemCharacterSetPtr = &newValue
	return o
}
