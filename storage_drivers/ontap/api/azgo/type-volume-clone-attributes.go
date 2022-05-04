// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeCloneAttributesType is a structure to represent a volume-clone-attributes ZAPI object
type VolumeCloneAttributesType struct {
	XMLName                        xml.Name                         `xml:"volume-clone-attributes"`
	CloneChildCountPtr             *int                             `xml:"clone-child-count"`
	VolumeCloneParentAttributesPtr *VolumeCloneParentAttributesType `xml:"volume-clone-parent-attributes"`
}

// NewVolumeCloneAttributesType is a factory method for creating new instances of VolumeCloneAttributesType objects
func NewVolumeCloneAttributesType() *VolumeCloneAttributesType {
	return &VolumeCloneAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CloneChildCount is a 'getter' method
func (o *VolumeCloneAttributesType) CloneChildCount() int {
	r := *o.CloneChildCountPtr
	return r
}

// SetCloneChildCount is a fluent style 'setter' method that can be chained
func (o *VolumeCloneAttributesType) SetCloneChildCount(newValue int) *VolumeCloneAttributesType {
	o.CloneChildCountPtr = &newValue
	return o
}

// VolumeCloneParentAttributes is a 'getter' method
func (o *VolumeCloneAttributesType) VolumeCloneParentAttributes() VolumeCloneParentAttributesType {
	r := *o.VolumeCloneParentAttributesPtr
	return r
}

// SetVolumeCloneParentAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeCloneAttributesType) SetVolumeCloneParentAttributes(newValue VolumeCloneParentAttributesType) *VolumeCloneAttributesType {
	o.VolumeCloneParentAttributesPtr = &newValue
	return o
}
