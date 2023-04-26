// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeInterfaceInfoType is a structure to represent a nvme-interface-info ZAPI object
type NvmeInterfaceInfoType struct {
	XMLName               xml.Name                                 `xml:"nvme-interface-info"`
	CommentPtr            *string                                  `xml:"comment"`
	FcWwnnPtr             *string                                  `xml:"fc-wwnn"`
	FcWwpnPtr             *string                                  `xml:"fc-wwpn"`
	HomeNodePtr           *FilerIdType                             `xml:"home-node"`
	HomePortPtr           *LifBindableType                         `xml:"home-port"`
	LifPtr                *LifNameType                             `xml:"lif"`
	LifUuidPtr            *UuidType                                `xml:"lif-uuid"`
	PhysicalProtocolPtr   *NvmePhysicalProtocolType                `xml:"physical-protocol"`
	StatusAdminPtr        *VifStatusType                           `xml:"status-admin"`
	TransportAddressPtr   *string                                  `xml:"transport-address"`
	TransportProtocolsPtr *NvmeInterfaceInfoTypeTransportProtocols `xml:"transport-protocols"`
	// work in progress
	VserverPtr     *VserverNameType `xml:"vserver"`
	VserverUuidPtr *UuidType        `xml:"vserver-uuid"`
}

// NewNvmeInterfaceInfoType is a factory method for creating new instances of NvmeInterfaceInfoType objects
func NewNvmeInterfaceInfoType() *NvmeInterfaceInfoType {
	return &NvmeInterfaceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeInterfaceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Comment is a 'getter' method
func (o *NvmeInterfaceInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetComment(newValue string) *NvmeInterfaceInfoType {
	o.CommentPtr = &newValue
	return o
}

// FcWwnn is a 'getter' method
func (o *NvmeInterfaceInfoType) FcWwnn() string {
	var r string
	if o.FcWwnnPtr == nil {
		return r
	}
	r = *o.FcWwnnPtr
	return r
}

// SetFcWwnn is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetFcWwnn(newValue string) *NvmeInterfaceInfoType {
	o.FcWwnnPtr = &newValue
	return o
}

// FcWwpn is a 'getter' method
func (o *NvmeInterfaceInfoType) FcWwpn() string {
	var r string
	if o.FcWwpnPtr == nil {
		return r
	}
	r = *o.FcWwpnPtr
	return r
}

// SetFcWwpn is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetFcWwpn(newValue string) *NvmeInterfaceInfoType {
	o.FcWwpnPtr = &newValue
	return o
}

// HomeNode is a 'getter' method
func (o *NvmeInterfaceInfoType) HomeNode() FilerIdType {
	var r FilerIdType
	if o.HomeNodePtr == nil {
		return r
	}
	r = *o.HomeNodePtr
	return r
}

// SetHomeNode is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetHomeNode(newValue FilerIdType) *NvmeInterfaceInfoType {
	o.HomeNodePtr = &newValue
	return o
}

// HomePort is a 'getter' method
func (o *NvmeInterfaceInfoType) HomePort() LifBindableType {
	var r LifBindableType
	if o.HomePortPtr == nil {
		return r
	}
	r = *o.HomePortPtr
	return r
}

// SetHomePort is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetHomePort(newValue LifBindableType) *NvmeInterfaceInfoType {
	o.HomePortPtr = &newValue
	return o
}

// Lif is a 'getter' method
func (o *NvmeInterfaceInfoType) Lif() LifNameType {
	var r LifNameType
	if o.LifPtr == nil {
		return r
	}
	r = *o.LifPtr
	return r
}

// SetLif is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetLif(newValue LifNameType) *NvmeInterfaceInfoType {
	o.LifPtr = &newValue
	return o
}

// LifUuid is a 'getter' method
func (o *NvmeInterfaceInfoType) LifUuid() UuidType {
	var r UuidType
	if o.LifUuidPtr == nil {
		return r
	}
	r = *o.LifUuidPtr
	return r
}

// SetLifUuid is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetLifUuid(newValue UuidType) *NvmeInterfaceInfoType {
	o.LifUuidPtr = &newValue
	return o
}

// PhysicalProtocol is a 'getter' method
func (o *NvmeInterfaceInfoType) PhysicalProtocol() NvmePhysicalProtocolType {
	var r NvmePhysicalProtocolType
	if o.PhysicalProtocolPtr == nil {
		return r
	}
	r = *o.PhysicalProtocolPtr
	return r
}

// SetPhysicalProtocol is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetPhysicalProtocol(newValue NvmePhysicalProtocolType) *NvmeInterfaceInfoType {
	o.PhysicalProtocolPtr = &newValue
	return o
}

// StatusAdmin is a 'getter' method
func (o *NvmeInterfaceInfoType) StatusAdmin() VifStatusType {
	var r VifStatusType
	if o.StatusAdminPtr == nil {
		return r
	}
	r = *o.StatusAdminPtr
	return r
}

// SetStatusAdmin is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetStatusAdmin(newValue VifStatusType) *NvmeInterfaceInfoType {
	o.StatusAdminPtr = &newValue
	return o
}

// TransportAddress is a 'getter' method
func (o *NvmeInterfaceInfoType) TransportAddress() string {
	var r string
	if o.TransportAddressPtr == nil {
		return r
	}
	r = *o.TransportAddressPtr
	return r
}

// SetTransportAddress is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetTransportAddress(newValue string) *NvmeInterfaceInfoType {
	o.TransportAddressPtr = &newValue
	return o
}

// NvmeInterfaceInfoTypeTransportProtocols is a wrapper
type NvmeInterfaceInfoTypeTransportProtocols struct {
	XMLName          xml.Name            `xml:"transport-protocols"`
	NvmeTransportPtr []NvmeTransportType `xml:"nvme-transport"`
}

// NvmeTransport is a 'getter' method
func (o *NvmeInterfaceInfoTypeTransportProtocols) NvmeTransport() []NvmeTransportType {
	r := o.NvmeTransportPtr
	return r
}

// SetNvmeTransport is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoTypeTransportProtocols) SetNvmeTransport(newValue []NvmeTransportType) *NvmeInterfaceInfoTypeTransportProtocols {
	newSlice := make([]NvmeTransportType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTransportPtr = newSlice
	return o
}

// TransportProtocols is a 'getter' method
func (o *NvmeInterfaceInfoType) TransportProtocols() NvmeInterfaceInfoTypeTransportProtocols {
	var r NvmeInterfaceInfoTypeTransportProtocols
	if o.TransportProtocolsPtr == nil {
		return r
	}
	r = *o.TransportProtocolsPtr
	return r
}

// SetTransportProtocols is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetTransportProtocols(newValue NvmeInterfaceInfoTypeTransportProtocols) *NvmeInterfaceInfoType {
	o.TransportProtocolsPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeInterfaceInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetVserver(newValue VserverNameType) *NvmeInterfaceInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeInterfaceInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceInfoType) SetVserverUuid(newValue UuidType) *NvmeInterfaceInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
