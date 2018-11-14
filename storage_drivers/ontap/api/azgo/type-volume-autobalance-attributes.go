package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeAutobalanceAttributesType is a structure to represent a volume-autobalance-attributes ZAPI object
type VolumeAutobalanceAttributesType struct {
	XMLName                  xml.Name `xml:"volume-autobalance-attributes"`
	IsAutobalanceEligiblePtr *bool    `xml:"is-autobalance-eligible"`
}

// NewVolumeAutobalanceAttributesType is a factory method for creating new instances of VolumeAutobalanceAttributesType objects
func NewVolumeAutobalanceAttributesType() *VolumeAutobalanceAttributesType {
	return &VolumeAutobalanceAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeAutobalanceAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeAutobalanceAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsAutobalanceEligible is a 'getter' method
func (o *VolumeAutobalanceAttributesType) IsAutobalanceEligible() bool {
	r := *o.IsAutobalanceEligiblePtr
	return r
}

// SetIsAutobalanceEligible is a fluent style 'setter' method that can be chained
func (o *VolumeAutobalanceAttributesType) SetIsAutobalanceEligible(newValue bool) *VolumeAutobalanceAttributesType {
	o.IsAutobalanceEligiblePtr = &newValue
	return o
}
