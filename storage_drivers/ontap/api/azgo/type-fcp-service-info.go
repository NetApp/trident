package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// FcpServiceInfoType is a structure to represent a fcp-service-info ZAPI object
type FcpServiceInfoType struct {
	XMLName        xml.Name `xml:"fcp-service-info"`
	IsAvailablePtr *bool    `xml:"is-available"`
	NodeNamePtr    *string  `xml:"node-name"`
	VserverPtr     *string  `xml:"vserver"`
}

// NewFcpServiceInfoType is a factory method for creating new instances of FcpServiceInfoType objects
func NewFcpServiceInfoType() *FcpServiceInfoType {
	return &FcpServiceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *FcpServiceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsAvailable is a 'getter' method
func (o *FcpServiceInfoType) IsAvailable() bool {
	var r bool
	if o.IsAvailablePtr == nil {
		return r
	}
	r = *o.IsAvailablePtr

	return r
}

// SetIsAvailable is a fluent style 'setter' method that can be chained
func (o *FcpServiceInfoType) SetIsAvailable(newValue bool) *FcpServiceInfoType {
	o.IsAvailablePtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *FcpServiceInfoType) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr

	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *FcpServiceInfoType) SetNodeName(newValue string) *FcpServiceInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *FcpServiceInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr

	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *FcpServiceInfoType) SetVserver(newValue string) *FcpServiceInfoType {
	o.VserverPtr = &newValue
	return o
}
