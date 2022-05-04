// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeExportAttributesType is a structure to represent a volume-export-attributes ZAPI object
type VolumeExportAttributesType struct {
	XMLName   xml.Name `xml:"volume-export-attributes"`
	PolicyPtr *string  `xml:"policy"`
}

// NewVolumeExportAttributesType is a factory method for creating new instances of VolumeExportAttributesType objects
func NewVolumeExportAttributesType() *VolumeExportAttributesType {
	return &VolumeExportAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeExportAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeExportAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Policy is a 'getter' method
func (o *VolumeExportAttributesType) Policy() string {
	r := *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeExportAttributesType) SetPolicy(newValue string) *VolumeExportAttributesType {
	o.PolicyPtr = &newValue
	return o
}
