// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrSnapmirrorAttributesType is a structure to represent a aggr-snapmirror-attributes ZAPI object
type AggrSnapmirrorAttributesType struct {
	XMLName                     xml.Name `xml:"aggr-snapmirror-attributes"`
	DpSnapmirrorDestinationsPtr *int     `xml:"dp-snapmirror-destinations"`
	LsSnapmirrorDestinationsPtr *int     `xml:"ls-snapmirror-destinations"`
	MvSnapmirrorDestinationsPtr *int     `xml:"mv-snapmirror-destinations"`
}

// NewAggrSnapmirrorAttributesType is a factory method for creating new instances of AggrSnapmirrorAttributesType objects
func NewAggrSnapmirrorAttributesType() *AggrSnapmirrorAttributesType {
	return &AggrSnapmirrorAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrSnapmirrorAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSnapmirrorAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// DpSnapmirrorDestinations is a 'getter' method
func (o *AggrSnapmirrorAttributesType) DpSnapmirrorDestinations() int {
	r := *o.DpSnapmirrorDestinationsPtr
	return r
}

// SetDpSnapmirrorDestinations is a fluent style 'setter' method that can be chained
func (o *AggrSnapmirrorAttributesType) SetDpSnapmirrorDestinations(newValue int) *AggrSnapmirrorAttributesType {
	o.DpSnapmirrorDestinationsPtr = &newValue
	return o
}

// LsSnapmirrorDestinations is a 'getter' method
func (o *AggrSnapmirrorAttributesType) LsSnapmirrorDestinations() int {
	r := *o.LsSnapmirrorDestinationsPtr
	return r
}

// SetLsSnapmirrorDestinations is a fluent style 'setter' method that can be chained
func (o *AggrSnapmirrorAttributesType) SetLsSnapmirrorDestinations(newValue int) *AggrSnapmirrorAttributesType {
	o.LsSnapmirrorDestinationsPtr = &newValue
	return o
}

// MvSnapmirrorDestinations is a 'getter' method
func (o *AggrSnapmirrorAttributesType) MvSnapmirrorDestinations() int {
	r := *o.MvSnapmirrorDestinationsPtr
	return r
}

// SetMvSnapmirrorDestinations is a fluent style 'setter' method that can be chained
func (o *AggrSnapmirrorAttributesType) SetMvSnapmirrorDestinations(newValue int) *AggrSnapmirrorAttributesType {
	o.MvSnapmirrorDestinationsPtr = &newValue
	return o
}
