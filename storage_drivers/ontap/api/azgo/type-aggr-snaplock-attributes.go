// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrSnaplockAttributesType is a structure to represent a aggr-snaplock-attributes ZAPI object
type AggrSnaplockAttributesType struct {
	XMLName         xml.Name `xml:"aggr-snaplock-attributes"`
	IsSnaplockPtr   *bool    `xml:"is-snaplock"`
	SnaplockTypePtr *string  `xml:"snaplock-type"`
}

// NewAggrSnaplockAttributesType is a factory method for creating new instances of AggrSnaplockAttributesType objects
func NewAggrSnaplockAttributesType() *AggrSnaplockAttributesType {
	return &AggrSnaplockAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrSnaplockAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSnaplockAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsSnaplock is a 'getter' method
func (o *AggrSnaplockAttributesType) IsSnaplock() bool {
	r := *o.IsSnaplockPtr
	return r
}

// SetIsSnaplock is a fluent style 'setter' method that can be chained
func (o *AggrSnaplockAttributesType) SetIsSnaplock(newValue bool) *AggrSnaplockAttributesType {
	o.IsSnaplockPtr = &newValue
	return o
}

// SnaplockType is a 'getter' method
func (o *AggrSnaplockAttributesType) SnaplockType() string {
	r := *o.SnaplockTypePtr
	return r
}

// SetSnaplockType is a fluent style 'setter' method that can be chained
func (o *AggrSnaplockAttributesType) SetSnaplockType(newValue string) *AggrSnaplockAttributesType {
	o.SnaplockTypePtr = &newValue
	return o
}
