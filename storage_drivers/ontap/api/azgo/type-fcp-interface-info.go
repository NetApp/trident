package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// FcpInterfaceInfoType is a structure to represent a fcp-interface-info ZAPI object
type FcpInterfaceInfoType struct {
	XMLName           xml.Name `xml:"fcp-interface-info"`
	CurrentNodePtr    *string  `xml:"current-node"`
	CurrentPortPtr    *string  `xml:"current-port"`
	InterfaceNamePtr  *string  `xml:"interface-name"`
	NodeNamePtr       *string  `xml:"node-name"`
	PortAddressPtr    *string  `xml:"port-address"`
	PortNamePtr       *string  `xml:"port-name"`
	RelativePortIdPtr *int     `xml:"relative-port-id"`
	VserverPtr        *string  `xml:"vserver"`
}

// NewFcpInterfaceInfoType is a factory method for creating new instances of FcpInterfaceInfoType objects
func NewFcpInterfaceInfoType() *FcpInterfaceInfoType {
	return &FcpInterfaceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *FcpInterfaceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CurrentNode is a 'getter' method
func (o *FcpInterfaceInfoType) CurrentNode() string {
	var r string
	if o.CurrentNodePtr == nil {
		return r
	}
	r = *o.CurrentNodePtr

	return r
}

// SetCurrentNode is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetCurrentNode(newValue string) *FcpInterfaceInfoType {
	o.CurrentNodePtr = &newValue
	return o
}

// CurrentPort is a 'getter' method
func (o *FcpInterfaceInfoType) CurrentPort() string {
	var r string
	if o.CurrentPortPtr == nil {
		return r
	}
	r = *o.CurrentPortPtr

	return r
}

// SetCurrentPort is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetCurrentPort(newValue string) *FcpInterfaceInfoType {
	o.CurrentPortPtr = &newValue
	return o
}

// InterfaceName is a 'getter' method
func (o *FcpInterfaceInfoType) InterfaceName() string {
	var r string
	if o.InterfaceNamePtr == nil {
		return r
	}
	r = *o.InterfaceNamePtr

	return r
}

// SetInterfaceName is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetInterfaceName(newValue string) *FcpInterfaceInfoType {
	o.InterfaceNamePtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *FcpInterfaceInfoType) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr

	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetNodeName(newValue string) *FcpInterfaceInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// PortAddress is a 'getter' method
func (o *FcpInterfaceInfoType) PortAddress() string {
	var r string
	if o.PortAddressPtr == nil {
		return r
	}
	r = *o.PortAddressPtr

	return r
}

// SetPortAddress is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetPortAddress(newValue string) *FcpInterfaceInfoType {
	o.PortAddressPtr = &newValue
	return o
}

// PortName is a 'getter' method
func (o *FcpInterfaceInfoType) PortName() string {
	var r string
	if o.PortNamePtr == nil {
		return r
	}
	r = *o.PortNamePtr

	return r
}

// SetPortName is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetPortName(newValue string) *FcpInterfaceInfoType {
	o.PortNamePtr = &newValue
	return o
}

// RelativePortId is a 'getter' method
func (o *FcpInterfaceInfoType) RelativePortId() int {
	var r int
	if o.RelativePortIdPtr == nil {
		return r
	}
	r = *o.RelativePortIdPtr

	return r
}

// SetRelativePortId is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetRelativePortId(newValue int) *FcpInterfaceInfoType {
	o.RelativePortIdPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *FcpInterfaceInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr

	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceInfoType) SetVserver(newValue string) *FcpInterfaceInfoType {
	o.VserverPtr = &newValue
	return o
}
