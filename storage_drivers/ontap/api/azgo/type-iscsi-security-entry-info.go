// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiSecurityEntryInfoType is a structure to represent a iscsi-security-entry-info ZAPI object
type IscsiSecurityEntryInfoType struct {
	XMLName                   xml.Name                                          `xml:"iscsi-security-entry-info"`
	AuthChapPolicyPtr         *string                                           `xml:"auth-chap-policy"`
	AuthTypePtr               *string                                           `xml:"auth-type"`
	InitiatorPtr              *string                                           `xml:"initiator"`
	InitiatorAddressRangesPtr *IscsiSecurityEntryInfoTypeInitiatorAddressRanges `xml:"initiator-address-ranges"`
	// work in progress
	OutboundUserNamePtr *string `xml:"outbound-user-name"`
	UserNamePtr         *string `xml:"user-name"`
	VserverPtr          *string `xml:"vserver"`
}

// NewIscsiSecurityEntryInfoType is a factory method for creating new instances of IscsiSecurityEntryInfoType objects
func NewIscsiSecurityEntryInfoType() *IscsiSecurityEntryInfoType {
	return &IscsiSecurityEntryInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiSecurityEntryInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiSecurityEntryInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AuthChapPolicy is a 'getter' method
func (o *IscsiSecurityEntryInfoType) AuthChapPolicy() string {
	var r string
	if o.AuthChapPolicyPtr == nil {
		return r
	}
	r = *o.AuthChapPolicyPtr
	return r
}

// SetAuthChapPolicy is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetAuthChapPolicy(newValue string) *IscsiSecurityEntryInfoType {
	o.AuthChapPolicyPtr = &newValue
	return o
}

// AuthType is a 'getter' method
func (o *IscsiSecurityEntryInfoType) AuthType() string {
	var r string
	if o.AuthTypePtr == nil {
		return r
	}
	r = *o.AuthTypePtr
	return r
}

// SetAuthType is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetAuthType(newValue string) *IscsiSecurityEntryInfoType {
	o.AuthTypePtr = &newValue
	return o
}

// Initiator is a 'getter' method
func (o *IscsiSecurityEntryInfoType) Initiator() string {
	var r string
	if o.InitiatorPtr == nil {
		return r
	}
	r = *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetInitiator(newValue string) *IscsiSecurityEntryInfoType {
	o.InitiatorPtr = &newValue
	return o
}

// IscsiSecurityEntryInfoTypeInitiatorAddressRanges is a wrapper
type IscsiSecurityEntryInfoTypeInitiatorAddressRanges struct {
	XMLName          xml.Name            `xml:"initiator-address-ranges"`
	IpRangeOrMaskPtr []IpRangeOrMaskType `xml:"ip-range-or-mask"`
}

// IpRangeOrMask is a 'getter' method
func (o *IscsiSecurityEntryInfoTypeInitiatorAddressRanges) IpRangeOrMask() []IpRangeOrMaskType {
	r := o.IpRangeOrMaskPtr
	return r
}

// SetIpRangeOrMask is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoTypeInitiatorAddressRanges) SetIpRangeOrMask(newValue []IpRangeOrMaskType) *IscsiSecurityEntryInfoTypeInitiatorAddressRanges {
	newSlice := make([]IpRangeOrMaskType, len(newValue))
	copy(newSlice, newValue)
	o.IpRangeOrMaskPtr = newSlice
	return o
}

// InitiatorAddressRanges is a 'getter' method
func (o *IscsiSecurityEntryInfoType) InitiatorAddressRanges() IscsiSecurityEntryInfoTypeInitiatorAddressRanges {
	var r IscsiSecurityEntryInfoTypeInitiatorAddressRanges
	if o.InitiatorAddressRangesPtr == nil {
		return r
	}
	r = *o.InitiatorAddressRangesPtr
	return r
}

// SetInitiatorAddressRanges is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetInitiatorAddressRanges(newValue IscsiSecurityEntryInfoTypeInitiatorAddressRanges) *IscsiSecurityEntryInfoType {
	o.InitiatorAddressRangesPtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiSecurityEntryInfoType) OutboundUserName() string {
	var r string
	if o.OutboundUserNamePtr == nil {
		return r
	}
	r = *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetOutboundUserName(newValue string) *IscsiSecurityEntryInfoType {
	o.OutboundUserNamePtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiSecurityEntryInfoType) UserName() string {
	var r string
	if o.UserNamePtr == nil {
		return r
	}
	r = *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetUserName(newValue string) *IscsiSecurityEntryInfoType {
	o.UserNamePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *IscsiSecurityEntryInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *IscsiSecurityEntryInfoType) SetVserver(newValue string) *IscsiSecurityEntryInfoType {
	o.VserverPtr = &newValue
	return o
}
