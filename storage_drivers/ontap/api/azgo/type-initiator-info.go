// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// InitiatorInfoType is a structure to represent a initiator-info ZAPI object
type InitiatorInfoType struct {
	XMLName          xml.Name `xml:"initiator-info"`
	InitiatorNamePtr *string  `xml:"initiator-name"`
}

// NewInitiatorInfoType is a factory method for creating new instances of InitiatorInfoType objects
func NewInitiatorInfoType() *InitiatorInfoType {
	return &InitiatorInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *InitiatorInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o InitiatorInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorName is a 'getter' method
func (o *InitiatorInfoType) InitiatorName() string {
	r := *o.InitiatorNamePtr
	return r
}

// SetInitiatorName is a fluent style 'setter' method that can be chained
func (o *InitiatorInfoType) SetInitiatorName(newValue string) *InitiatorInfoType {
	o.InitiatorNamePtr = &newValue
	return o
}
