// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrStatusAttributesType is a structure to represent a aggr-status-attributes ZAPI object
type AggrStatusAttributesType struct {
	XMLName                     xml.Name `xml:"aggr-status-attributes"`
	Is64BitUpgradeInProgressPtr *bool    `xml:"is-64-bit-upgrade-in-progress"`
}

// NewAggrStatusAttributesType is a factory method for creating new instances of AggrStatusAttributesType objects
func NewAggrStatusAttributesType() *AggrStatusAttributesType {
	return &AggrStatusAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrStatusAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrStatusAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Is64BitUpgradeInProgress is a 'getter' method
func (o *AggrStatusAttributesType) Is64BitUpgradeInProgress() bool {
	r := *o.Is64BitUpgradeInProgressPtr
	return r
}

// SetIs64BitUpgradeInProgress is a fluent style 'setter' method that can be chained
func (o *AggrStatusAttributesType) SetIs64BitUpgradeInProgress(newValue bool) *AggrStatusAttributesType {
	o.Is64BitUpgradeInProgressPtr = &newValue
	return o
}
