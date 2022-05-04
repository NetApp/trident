// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// InitiatorGroupListInfoType is a structure to represent a initiator-group-list-info ZAPI object
type InitiatorGroupListInfoType struct {
	XMLName               xml.Name `xml:"initiator-group-list-info"`
	InitiatorGroupNamePtr *string  `xml:"initiator-group-name"`
}

// NewInitiatorGroupListInfoType is a factory method for creating new instances of InitiatorGroupListInfoType objects
func NewInitiatorGroupListInfoType() *InitiatorGroupListInfoType {
	return &InitiatorGroupListInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *InitiatorGroupListInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o InitiatorGroupListInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupName is a 'getter' method
func (o *InitiatorGroupListInfoType) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupListInfoType) SetInitiatorGroupName(newValue string) *InitiatorGroupListInfoType {
	o.InitiatorGroupNamePtr = &newValue
	return o
}
