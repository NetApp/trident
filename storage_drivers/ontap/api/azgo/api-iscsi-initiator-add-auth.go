// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// IscsiInitiatorAddAuthRequest is a structure to represent a iscsi-initiator-add-auth Request ZAPI object
type IscsiInitiatorAddAuthRequest struct {
	XMLName                   xml.Name                                            `xml:"iscsi-initiator-add-auth"`
	AuthTypePtr               *string                                             `xml:"auth-type"`
	InitiatorPtr              *string                                             `xml:"initiator"`
	InitiatorAddressRangesPtr *IscsiInitiatorAddAuthRequestInitiatorAddressRanges `xml:"initiator-address-ranges"`
	OutboundPassphrasePtr     *string                                             `xml:"outbound-passphrase"`
	OutboundPasswordPtr       *string                                             `xml:"outbound-password"`
	OutboundUserNamePtr       *string                                             `xml:"outbound-user-name"`
	PassphrasePtr             *string                                             `xml:"passphrase"`
	PasswordPtr               *string                                             `xml:"password"`
	RadiusPtr                 *bool                                               `xml:"radius"`
	UserNamePtr               *string                                             `xml:"user-name"`
}

// IscsiInitiatorAddAuthResponse is a structure to represent a iscsi-initiator-add-auth Response ZAPI object
type IscsiInitiatorAddAuthResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          IscsiInitiatorAddAuthResponseResult `xml:"results"`
}

// NewIscsiInitiatorAddAuthResponse is a factory method for creating new instances of IscsiInitiatorAddAuthResponse objects
func NewIscsiInitiatorAddAuthResponse() *IscsiInitiatorAddAuthResponse {
	return &IscsiInitiatorAddAuthResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAddAuthResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAddAuthResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorAddAuthResponseResult is a structure to represent a iscsi-initiator-add-auth Response Result ZAPI object
type IscsiInitiatorAddAuthResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIscsiInitiatorAddAuthRequest is a factory method for creating new instances of IscsiInitiatorAddAuthRequest objects
func NewIscsiInitiatorAddAuthRequest() *IscsiInitiatorAddAuthRequest {
	return &IscsiInitiatorAddAuthRequest{}
}

// NewIscsiInitiatorAddAuthResponseResult is a factory method for creating new instances of IscsiInitiatorAddAuthResponseResult objects
func NewIscsiInitiatorAddAuthResponseResult() *IscsiInitiatorAddAuthResponseResult {
	return &IscsiInitiatorAddAuthResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAddAuthRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAddAuthResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAddAuthRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAddAuthResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorAddAuthRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorAddAuthResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorAddAuthRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorAddAuthResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorAddAuthRequest", NewIscsiInitiatorAddAuthResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorAddAuthResponse), err
}

// AuthType is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) AuthType() string {
	var r string
	if o.AuthTypePtr == nil {
		return r
	}
	r = *o.AuthTypePtr
	return r
}

// SetAuthType is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetAuthType(newValue string) *IscsiInitiatorAddAuthRequest {
	o.AuthTypePtr = &newValue
	return o
}

// Initiator is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) Initiator() string {
	var r string
	if o.InitiatorPtr == nil {
		return r
	}
	r = *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetInitiator(newValue string) *IscsiInitiatorAddAuthRequest {
	o.InitiatorPtr = &newValue
	return o
}

// IscsiInitiatorAddAuthRequestInitiatorAddressRanges is a wrapper
type IscsiInitiatorAddAuthRequestInitiatorAddressRanges struct {
	XMLName          xml.Name            `xml:"initiator-address-ranges"`
	IpRangeOrMaskPtr []IpRangeOrMaskType `xml:"ip-range-or-mask"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAddAuthRequestInitiatorAddressRanges) String() string {
	return ToString(reflect.ValueOf(o))
}

// IpRangeOrMask is a 'getter' method
func (o *IscsiInitiatorAddAuthRequestInitiatorAddressRanges) IpRangeOrMask() []IpRangeOrMaskType {
	r := o.IpRangeOrMaskPtr
	return r
}

// SetIpRangeOrMask is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequestInitiatorAddressRanges) SetIpRangeOrMask(newValue []IpRangeOrMaskType) *IscsiInitiatorAddAuthRequestInitiatorAddressRanges {
	newSlice := make([]IpRangeOrMaskType, len(newValue))
	copy(newSlice, newValue)
	o.IpRangeOrMaskPtr = newSlice
	return o
}

// InitiatorAddressRanges is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) InitiatorAddressRanges() IscsiInitiatorAddAuthRequestInitiatorAddressRanges {
	var r IscsiInitiatorAddAuthRequestInitiatorAddressRanges
	if o.InitiatorAddressRangesPtr == nil {
		return r
	}
	r = *o.InitiatorAddressRangesPtr
	return r
}

// SetInitiatorAddressRanges is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetInitiatorAddressRanges(newValue IscsiInitiatorAddAuthRequestInitiatorAddressRanges) *IscsiInitiatorAddAuthRequest {
	o.InitiatorAddressRangesPtr = &newValue
	return o
}

// OutboundPassphrase is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) OutboundPassphrase() string {
	var r string
	if o.OutboundPassphrasePtr == nil {
		return r
	}
	r = *o.OutboundPassphrasePtr
	return r
}

// SetOutboundPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetOutboundPassphrase(newValue string) *IscsiInitiatorAddAuthRequest {
	o.OutboundPassphrasePtr = &newValue
	return o
}

// OutboundPassword is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) OutboundPassword() string {
	var r string
	if o.OutboundPasswordPtr == nil {
		return r
	}
	r = *o.OutboundPasswordPtr
	return r
}

// SetOutboundPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetOutboundPassword(newValue string) *IscsiInitiatorAddAuthRequest {
	o.OutboundPasswordPtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) OutboundUserName() string {
	var r string
	if o.OutboundUserNamePtr == nil {
		return r
	}
	r = *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetOutboundUserName(newValue string) *IscsiInitiatorAddAuthRequest {
	o.OutboundUserNamePtr = &newValue
	return o
}

// Passphrase is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) Passphrase() string {
	var r string
	if o.PassphrasePtr == nil {
		return r
	}
	r = *o.PassphrasePtr
	return r
}

// SetPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetPassphrase(newValue string) *IscsiInitiatorAddAuthRequest {
	o.PassphrasePtr = &newValue
	return o
}

// Password is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) Password() string {
	var r string
	if o.PasswordPtr == nil {
		return r
	}
	r = *o.PasswordPtr
	return r
}

// SetPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetPassword(newValue string) *IscsiInitiatorAddAuthRequest {
	o.PasswordPtr = &newValue
	return o
}

// Radius is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) Radius() bool {
	var r bool
	if o.RadiusPtr == nil {
		return r
	}
	r = *o.RadiusPtr
	return r
}

// SetRadius is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetRadius(newValue bool) *IscsiInitiatorAddAuthRequest {
	o.RadiusPtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiInitiatorAddAuthRequest) UserName() string {
	var r string
	if o.UserNamePtr == nil {
		return r
	}
	r = *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAddAuthRequest) SetUserName(newValue string) *IscsiInitiatorAddAuthRequest {
	o.UserNamePtr = &newValue
	return o
}
