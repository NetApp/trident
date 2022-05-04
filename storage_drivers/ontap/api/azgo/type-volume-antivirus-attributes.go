// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeAntivirusAttributesType is a structure to represent a volume-antivirus-attributes ZAPI object
type VolumeAntivirusAttributesType struct {
	XMLName           xml.Name `xml:"volume-antivirus-attributes"`
	OnAccessPolicyPtr *string  `xml:"on-access-policy"`
}

// NewVolumeAntivirusAttributesType is a factory method for creating new instances of VolumeAntivirusAttributesType objects
func NewVolumeAntivirusAttributesType() *VolumeAntivirusAttributesType {
	return &VolumeAntivirusAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeAntivirusAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeAntivirusAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// OnAccessPolicy is a 'getter' method
func (o *VolumeAntivirusAttributesType) OnAccessPolicy() string {
	r := *o.OnAccessPolicyPtr
	return r
}

// SetOnAccessPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeAntivirusAttributesType) SetOnAccessPolicy(newValue string) *VolumeAntivirusAttributesType {
	o.OnAccessPolicyPtr = &newValue
	return o
}
