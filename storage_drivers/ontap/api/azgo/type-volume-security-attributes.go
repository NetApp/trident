// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSecurityAttributesType is a structure to represent a volume-security-attributes ZAPI object
type VolumeSecurityAttributesType struct {
	XMLName                         xml.Name                          `xml:"volume-security-attributes"`
	StylePtr                        *string                           `xml:"style"`
	VolumeSecurityUnixAttributesPtr *VolumeSecurityUnixAttributesType `xml:"volume-security-unix-attributes"`
}

// NewVolumeSecurityAttributesType is a factory method for creating new instances of VolumeSecurityAttributesType objects
func NewVolumeSecurityAttributesType() *VolumeSecurityAttributesType {
	return &VolumeSecurityAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSecurityAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSecurityAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Style is a 'getter' method
func (o *VolumeSecurityAttributesType) Style() string {
	r := *o.StylePtr
	return r
}

// SetStyle is a fluent style 'setter' method that can be chained
func (o *VolumeSecurityAttributesType) SetStyle(newValue string) *VolumeSecurityAttributesType {
	o.StylePtr = &newValue
	return o
}

// VolumeSecurityUnixAttributes is a 'getter' method
func (o *VolumeSecurityAttributesType) VolumeSecurityUnixAttributes() VolumeSecurityUnixAttributesType {
	r := *o.VolumeSecurityUnixAttributesPtr
	return r
}

// SetVolumeSecurityUnixAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeSecurityAttributesType) SetVolumeSecurityUnixAttributes(newValue VolumeSecurityUnixAttributesType) *VolumeSecurityAttributesType {
	o.VolumeSecurityUnixAttributesPtr = &newValue
	return o
}
