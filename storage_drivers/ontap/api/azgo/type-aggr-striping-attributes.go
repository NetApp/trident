// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrStripingAttributesType is a structure to represent a aggr-striping-attributes ZAPI object
type AggrStripingAttributesType struct {
	XMLName        xml.Name `xml:"aggr-striping-attributes"`
	MemberCountPtr *int     `xml:"member-count"`
}

// NewAggrStripingAttributesType is a factory method for creating new instances of AggrStripingAttributesType objects
func NewAggrStripingAttributesType() *AggrStripingAttributesType {
	return &AggrStripingAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrStripingAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrStripingAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// MemberCount is a 'getter' method
func (o *AggrStripingAttributesType) MemberCount() int {
	r := *o.MemberCountPtr
	return r
}

// SetMemberCount is a fluent style 'setter' method that can be chained
func (o *AggrStripingAttributesType) SetMemberCount(newValue int) *AggrStripingAttributesType {
	o.MemberCountPtr = &newValue
	return o
}
