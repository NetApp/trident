package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrStartAttributesType is a structure to represent a aggr-start-attributes ZAPI object
type AggrStartAttributesType struct {
	XMLName               xml.Name `xml:"aggr-start-attributes"`
	MinSpaceForUpgradePtr *int     `xml:"min-space-for-upgrade"`
	StartLastErrnoPtr     *int     `xml:"start-last-errno"`
}

// NewAggrStartAttributesType is a factory method for creating new instances of AggrStartAttributesType objects
func NewAggrStartAttributesType() *AggrStartAttributesType {
	return &AggrStartAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrStartAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrStartAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// MinSpaceForUpgrade is a 'getter' method
func (o *AggrStartAttributesType) MinSpaceForUpgrade() int {
	r := *o.MinSpaceForUpgradePtr
	return r
}

// SetMinSpaceForUpgrade is a fluent style 'setter' method that can be chained
func (o *AggrStartAttributesType) SetMinSpaceForUpgrade(newValue int) *AggrStartAttributesType {
	o.MinSpaceForUpgradePtr = &newValue
	return o
}

// StartLastErrno is a 'getter' method
func (o *AggrStartAttributesType) StartLastErrno() int {
	r := *o.StartLastErrnoPtr
	return r
}

// SetStartLastErrno is a fluent style 'setter' method that can be chained
func (o *AggrStartAttributesType) SetStartLastErrno(newValue int) *AggrStartAttributesType {
	o.StartLastErrnoPtr = &newValue
	return o
}
