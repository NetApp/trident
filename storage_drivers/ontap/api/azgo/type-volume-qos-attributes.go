package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeQosAttributesType is a structure to represent a volume-qos-attributes ZAPI object
type VolumeQosAttributesType struct {
	XMLName                    xml.Name `xml:"volume-qos-attributes"`
	AdaptivePolicyGroupNamePtr *string  `xml:"adaptive-policy-group-name"`
	PolicyGroupNamePtr         *string  `xml:"policy-group-name"`
}

// NewVolumeQosAttributesType is a factory method for creating new instances of VolumeQosAttributesType objects
func NewVolumeQosAttributesType() *VolumeQosAttributesType {
	return &VolumeQosAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeQosAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeQosAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AdaptivePolicyGroupName is a 'getter' method
func (o *VolumeQosAttributesType) AdaptivePolicyGroupName() string {
	r := *o.AdaptivePolicyGroupNamePtr
	return r
}

// SetAdaptivePolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeQosAttributesType) SetAdaptivePolicyGroupName(newValue string) *VolumeQosAttributesType {
	o.AdaptivePolicyGroupNamePtr = &newValue
	return o
}

// PolicyGroupName is a 'getter' method
func (o *VolumeQosAttributesType) PolicyGroupName() string {
	r := *o.PolicyGroupNamePtr
	return r
}

// SetPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeQosAttributesType) SetPolicyGroupName(newValue string) *VolumeQosAttributesType {
	o.PolicyGroupNamePtr = &newValue
	return o
}
