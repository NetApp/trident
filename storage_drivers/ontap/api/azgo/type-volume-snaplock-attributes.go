// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSnaplockAttributesType is a structure to represent a volume-snaplock-attributes ZAPI object
type VolumeSnaplockAttributesType struct {
	XMLName         xml.Name          `xml:"volume-snaplock-attributes"`
	SnaplockTypePtr *SnaplocktypeType `xml:"snaplock-type"`
}

// NewVolumeSnaplockAttributesType is a factory method for creating new instances of VolumeSnaplockAttributesType objects
func NewVolumeSnaplockAttributesType() *VolumeSnaplockAttributesType {
	return &VolumeSnaplockAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSnaplockAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSnaplockAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnaplockType is a 'getter' method
func (o *VolumeSnaplockAttributesType) SnaplockType() SnaplocktypeType {
	r := *o.SnaplockTypePtr
	return r
}

// SetSnaplockType is a fluent style 'setter' method that can be chained
func (o *VolumeSnaplockAttributesType) SetSnaplockType(newValue SnaplocktypeType) *VolumeSnaplockAttributesType {
	o.SnaplockTypePtr = &newValue
	return o
}
