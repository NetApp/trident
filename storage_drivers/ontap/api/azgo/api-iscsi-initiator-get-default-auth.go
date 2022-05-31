// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// IscsiInitiatorGetDefaultAuthRequest is a structure to represent a iscsi-initiator-get-default-auth Request ZAPI object
type IscsiInitiatorGetDefaultAuthRequest struct {
	XMLName xml.Name `xml:"iscsi-initiator-get-default-auth"`
}

// IscsiInitiatorGetDefaultAuthResponse is a structure to represent a iscsi-initiator-get-default-auth Response ZAPI object
type IscsiInitiatorGetDefaultAuthResponse struct {
	XMLName         xml.Name                                   `xml:"netapp"`
	ResponseVersion string                                     `xml:"version,attr"`
	ResponseXmlns   string                                     `xml:"xmlns,attr"`
	Result          IscsiInitiatorGetDefaultAuthResponseResult `xml:"results"`
}

// NewIscsiInitiatorGetDefaultAuthResponse is a factory method for creating new instances of IscsiInitiatorGetDefaultAuthResponse objects
func NewIscsiInitiatorGetDefaultAuthResponse() *IscsiInitiatorGetDefaultAuthResponse {
	return &IscsiInitiatorGetDefaultAuthResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetDefaultAuthResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetDefaultAuthResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorGetDefaultAuthResponseResult is a structure to represent a iscsi-initiator-get-default-auth Response Result ZAPI object
type IscsiInitiatorGetDefaultAuthResponseResult struct {
	XMLName             xml.Name `xml:"results"`
	ResultStatusAttr    string   `xml:"status,attr"`
	ResultReasonAttr    string   `xml:"reason,attr"`
	ResultErrnoAttr     string   `xml:"errno,attr"`
	AuthChapPolicyPtr   *string  `xml:"auth-chap-policy"`
	AuthTypePtr         *string  `xml:"auth-type"`
	OutboundUserNamePtr *string  `xml:"outbound-user-name"`
	UserNamePtr         *string  `xml:"user-name"`
}

// NewIscsiInitiatorGetDefaultAuthRequest is a factory method for creating new instances of IscsiInitiatorGetDefaultAuthRequest objects
func NewIscsiInitiatorGetDefaultAuthRequest() *IscsiInitiatorGetDefaultAuthRequest {
	return &IscsiInitiatorGetDefaultAuthRequest{}
}

// NewIscsiInitiatorGetDefaultAuthResponseResult is a factory method for creating new instances of IscsiInitiatorGetDefaultAuthResponseResult objects
func NewIscsiInitiatorGetDefaultAuthResponseResult() *IscsiInitiatorGetDefaultAuthResponseResult {
	return &IscsiInitiatorGetDefaultAuthResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetDefaultAuthRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetDefaultAuthResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetDefaultAuthRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetDefaultAuthResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetDefaultAuthRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorGetDefaultAuthResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetDefaultAuthRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorGetDefaultAuthResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorGetDefaultAuthRequest", NewIscsiInitiatorGetDefaultAuthResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorGetDefaultAuthResponse), err
}

// AuthChapPolicy is a 'getter' method
func (o *IscsiInitiatorGetDefaultAuthResponseResult) AuthChapPolicy() string {
	var r string
	if o.AuthChapPolicyPtr == nil {
		return r
	}
	r = *o.AuthChapPolicyPtr
	return r
}

// SetAuthChapPolicy is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetDefaultAuthResponseResult) SetAuthChapPolicy(newValue string) *IscsiInitiatorGetDefaultAuthResponseResult {
	o.AuthChapPolicyPtr = &newValue
	return o
}

// AuthType is a 'getter' method
func (o *IscsiInitiatorGetDefaultAuthResponseResult) AuthType() string {
	var r string
	if o.AuthTypePtr == nil {
		return r
	}
	r = *o.AuthTypePtr
	return r
}

// SetAuthType is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetDefaultAuthResponseResult) SetAuthType(newValue string) *IscsiInitiatorGetDefaultAuthResponseResult {
	o.AuthTypePtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiInitiatorGetDefaultAuthResponseResult) OutboundUserName() string {
	var r string
	if o.OutboundUserNamePtr == nil {
		return r
	}
	r = *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetDefaultAuthResponseResult) SetOutboundUserName(newValue string) *IscsiInitiatorGetDefaultAuthResponseResult {
	o.OutboundUserNamePtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiInitiatorGetDefaultAuthResponseResult) UserName() string {
	var r string
	if o.UserNamePtr == nil {
		return r
	}
	r = *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetDefaultAuthResponseResult) SetUserName(newValue string) *IscsiInitiatorGetDefaultAuthResponseResult {
	o.UserNamePtr = &newValue
	return o
}
