package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeCompAggrAttributesType is a structure to represent a volume-comp-aggr-attributes ZAPI object
type VolumeCompAggrAttributesType struct {
	XMLName          xml.Name `xml:"volume-comp-aggr-attributes"`
	TieringPolicyPtr *string  `xml:"tiering-policy"`
}

// NewVolumeCompAggrAttributesType is a factory method for creating new instances of VolumeCompAggrAttributesType objects
func NewVolumeCompAggrAttributesType() *VolumeCompAggrAttributesType {
	return &VolumeCompAggrAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCompAggrAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCompAggrAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// TieringPolicy is a 'getter' method
func (o *VolumeCompAggrAttributesType) TieringPolicy() string {
	r := *o.TieringPolicyPtr
	return r
}

// SetTieringPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCompAggrAttributesType) SetTieringPolicy(newValue string) *VolumeCompAggrAttributesType {
	o.TieringPolicyPtr = &newValue
	return o
}
