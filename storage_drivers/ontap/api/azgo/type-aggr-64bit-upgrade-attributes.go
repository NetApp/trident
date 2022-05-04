// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// Aggr64bitUpgradeAttributesType is a structure to represent a aggr-64bit-upgrade-attributes ZAPI object
type Aggr64bitUpgradeAttributesType struct {
	XMLName                 xml.Name                  `xml:"aggr-64bit-upgrade-attributes"`
	AggrCheckAttributesPtr  *AggrCheckAttributesType  `xml:"aggr-check-attributes"`
	AggrStartAttributesPtr  *AggrStartAttributesType  `xml:"aggr-start-attributes"`
	AggrStatusAttributesPtr *AggrStatusAttributesType `xml:"aggr-status-attributes"`
}

// NewAggr64bitUpgradeAttributesType is a factory method for creating new instances of Aggr64bitUpgradeAttributesType objects
func NewAggr64bitUpgradeAttributesType() *Aggr64bitUpgradeAttributesType {
	return &Aggr64bitUpgradeAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *Aggr64bitUpgradeAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o Aggr64bitUpgradeAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrCheckAttributes is a 'getter' method
func (o *Aggr64bitUpgradeAttributesType) AggrCheckAttributes() AggrCheckAttributesType {
	r := *o.AggrCheckAttributesPtr
	return r
}

// SetAggrCheckAttributes is a fluent style 'setter' method that can be chained
func (o *Aggr64bitUpgradeAttributesType) SetAggrCheckAttributes(newValue AggrCheckAttributesType) *Aggr64bitUpgradeAttributesType {
	o.AggrCheckAttributesPtr = &newValue
	return o
}

// AggrStartAttributes is a 'getter' method
func (o *Aggr64bitUpgradeAttributesType) AggrStartAttributes() AggrStartAttributesType {
	r := *o.AggrStartAttributesPtr
	return r
}

// SetAggrStartAttributes is a fluent style 'setter' method that can be chained
func (o *Aggr64bitUpgradeAttributesType) SetAggrStartAttributes(newValue AggrStartAttributesType) *Aggr64bitUpgradeAttributesType {
	o.AggrStartAttributesPtr = &newValue
	return o
}

// AggrStatusAttributes is a 'getter' method
func (o *Aggr64bitUpgradeAttributesType) AggrStatusAttributes() AggrStatusAttributesType {
	r := *o.AggrStatusAttributesPtr
	return r
}

// SetAggrStatusAttributes is a fluent style 'setter' method that can be chained
func (o *Aggr64bitUpgradeAttributesType) SetAggrStatusAttributes(newValue AggrStatusAttributesType) *Aggr64bitUpgradeAttributesType {
	o.AggrStatusAttributesPtr = &newValue
	return o
}
