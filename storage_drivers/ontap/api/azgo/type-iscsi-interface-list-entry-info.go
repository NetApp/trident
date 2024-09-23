// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInterfaceListEntryInfoType is a structure to represent a iscsi-interface-list-entry-info ZAPI object
type IscsiInterfaceListEntryInfoType struct {
	XMLName               xml.Name `xml:"iscsi-interface-list-entry-info"`
	CurrentNodePtr        *string  `xml:"current-node"`
	CurrentPortPtr        *string  `xml:"current-port"`
	InterfaceNamePtr      *string  `xml:"interface-name"`
	IpAddressPtr          *string  `xml:"ip-address"`
	IpPortPtr             *int     `xml:"ip-port"`
	IsInterfaceEnabledPtr *bool    `xml:"is-interface-enabled"`
	RelativePortIdPtr     *int     `xml:"relative-port-id"`
	SendtargetsFqdnPtr    *string  `xml:"sendtargets-fqdn"`
	TpgroupNamePtr        *string  `xml:"tpgroup-name"`
	TpgroupTagPtr         *int     `xml:"tpgroup-tag"`
	VserverPtr            *string  `xml:"vserver"`
}

// NewIscsiInterfaceListEntryInfoType is a factory method for creating new instances of IscsiInterfaceListEntryInfoType objects
func NewIscsiInterfaceListEntryInfoType() *IscsiInterfaceListEntryInfoType {
	return &IscsiInterfaceListEntryInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInterfaceListEntryInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceListEntryInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CurrentNode is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) CurrentNode() string {
	var r string
	if o.CurrentNodePtr == nil {
		return r
	}
	r = *o.CurrentNodePtr
	return r
}

// SetCurrentNode is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetCurrentNode(newValue string) *IscsiInterfaceListEntryInfoType {
	o.CurrentNodePtr = &newValue
	return o
}

// CurrentPort is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) CurrentPort() string {
	var r string
	if o.CurrentPortPtr == nil {
		return r
	}
	r = *o.CurrentPortPtr
	return r
}

// SetCurrentPort is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetCurrentPort(newValue string) *IscsiInterfaceListEntryInfoType {
	o.CurrentPortPtr = &newValue
	return o
}

// InterfaceName is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) InterfaceName() string {
	var r string
	if o.InterfaceNamePtr == nil {
		return r
	}
	r = *o.InterfaceNamePtr
	return r
}

// SetInterfaceName is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetInterfaceName(newValue string) *IscsiInterfaceListEntryInfoType {
	o.InterfaceNamePtr = &newValue
	return o
}

// IpAddress is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) IpAddress() string {
	var r string
	if o.IpAddressPtr == nil {
		return r
	}
	r = *o.IpAddressPtr
	return r
}

// SetIpAddress is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetIpAddress(newValue string) *IscsiInterfaceListEntryInfoType {
	o.IpAddressPtr = &newValue
	return o
}

// IpPort is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) IpPort() int {
	var r int
	if o.IpPortPtr == nil {
		return r
	}
	r = *o.IpPortPtr
	return r
}

// SetIpPort is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetIpPort(newValue int) *IscsiInterfaceListEntryInfoType {
	o.IpPortPtr = &newValue
	return o
}

// IsInterfaceEnabled is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) IsInterfaceEnabled() bool {
	var r bool
	if o.IsInterfaceEnabledPtr == nil {
		return r
	}
	r = *o.IsInterfaceEnabledPtr
	return r
}

// SetIsInterfaceEnabled is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetIsInterfaceEnabled(newValue bool) *IscsiInterfaceListEntryInfoType {
	o.IsInterfaceEnabledPtr = &newValue
	return o
}

// RelativePortId is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) RelativePortId() int {
	var r int
	if o.RelativePortIdPtr == nil {
		return r
	}
	r = *o.RelativePortIdPtr
	return r
}

// SetRelativePortId is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetRelativePortId(newValue int) *IscsiInterfaceListEntryInfoType {
	o.RelativePortIdPtr = &newValue
	return o
}

// SendtargetsFqdn is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) SendtargetsFqdn() string {
	var r string
	if o.SendtargetsFqdnPtr == nil {
		return r
	}
	r = *o.SendtargetsFqdnPtr
	return r
}

// SetSendtargetsFqdn is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetSendtargetsFqdn(newValue string) *IscsiInterfaceListEntryInfoType {
	o.SendtargetsFqdnPtr = &newValue
	return o
}

// TpgroupName is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) TpgroupName() string {
	var r string
	if o.TpgroupNamePtr == nil {
		return r
	}
	r = *o.TpgroupNamePtr
	return r
}

// SetTpgroupName is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetTpgroupName(newValue string) *IscsiInterfaceListEntryInfoType {
	o.TpgroupNamePtr = &newValue
	return o
}

// TpgroupTag is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) TpgroupTag() int {
	var r int
	if o.TpgroupTagPtr == nil {
		return r
	}
	r = *o.TpgroupTagPtr
	return r
}

// SetTpgroupTag is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetTpgroupTag(newValue int) *IscsiInterfaceListEntryInfoType {
	o.TpgroupTagPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *IscsiInterfaceListEntryInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceListEntryInfoType) SetVserver(newValue string) *IscsiInterfaceListEntryInfoType {
	o.VserverPtr = &newValue
	return o
}
