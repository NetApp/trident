// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VserverGetRequest is a structure to represent a vserver-get Request ZAPI object
type VserverGetRequest struct {
	XMLName              xml.Name                            `xml:"vserver-get"`
	DesiredAttributesPtr *VserverGetRequestDesiredAttributes `xml:"desired-attributes"`
}

// VserverGetResponse is a structure to represent a vserver-get Response ZAPI object
type VserverGetResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          VserverGetResponseResult `xml:"results"`
}

// NewVserverGetResponse is a factory method for creating new instances of VserverGetResponse objects
func NewVserverGetResponse() *VserverGetResponse {
	return &VserverGetResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VserverGetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VserverGetResponseResult is a structure to represent a vserver-get Response Result ZAPI object
type VserverGetResponseResult struct {
	XMLName          xml.Name                            `xml:"results"`
	ResultStatusAttr string                              `xml:"status,attr"`
	ResultReasonAttr string                              `xml:"reason,attr"`
	ResultErrnoAttr  string                              `xml:"errno,attr"`
	AttributesPtr    *VserverGetResponseResultAttributes `xml:"attributes"`
}

// NewVserverGetRequest is a factory method for creating new instances of VserverGetRequest objects
func NewVserverGetRequest() *VserverGetRequest {
	return &VserverGetRequest{}
}

// NewVserverGetResponseResult is a factory method for creating new instances of VserverGetResponseResult objects
func NewVserverGetResponseResult() *VserverGetResponseResult {
	return &VserverGetResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VserverGetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VserverGetResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverGetRequest) ExecuteUsing(zr *ZapiRunner) (*VserverGetResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverGetRequest) executeWithoutIteration(zr *ZapiRunner) (*VserverGetResponse, error) {
	result, err := zr.ExecuteUsing(o, "VserverGetRequest", NewVserverGetResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VserverGetResponse), err
}

// VserverGetRequestDesiredAttributes is a wrapper
type VserverGetRequestDesiredAttributes struct {
	XMLName        xml.Name         `xml:"desired-attributes"`
	VserverInfoPtr *VserverInfoType `xml:"vserver-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfo is a 'getter' method
func (o *VserverGetRequestDesiredAttributes) VserverInfo() VserverInfoType {
	var r VserverInfoType
	if o.VserverInfoPtr == nil {
		return r
	}
	r = *o.VserverInfoPtr
	return r
}

// SetVserverInfo is a fluent style 'setter' method that can be chained
func (o *VserverGetRequestDesiredAttributes) SetVserverInfo(newValue VserverInfoType) *VserverGetRequestDesiredAttributes {
	o.VserverInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VserverGetRequest) DesiredAttributes() VserverGetRequestDesiredAttributes {
	var r VserverGetRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverGetRequest) SetDesiredAttributes(newValue VserverGetRequestDesiredAttributes) *VserverGetRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// VserverGetResponseResultAttributes is a wrapper
type VserverGetResponseResultAttributes struct {
	XMLName        xml.Name         `xml:"attributes"`
	VserverInfoPtr *VserverInfoType `xml:"vserver-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetResponseResultAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfo is a 'getter' method
func (o *VserverGetResponseResultAttributes) VserverInfo() VserverInfoType {
	var r VserverInfoType
	if o.VserverInfoPtr == nil {
		return r
	}
	r = *o.VserverInfoPtr
	return r
}

// SetVserverInfo is a fluent style 'setter' method that can be chained
func (o *VserverGetResponseResultAttributes) SetVserverInfo(newValue VserverInfoType) *VserverGetResponseResultAttributes {
	o.VserverInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *VserverGetResponseResultAttributes) values() VserverInfoType {
	var r VserverInfoType
	if o.VserverInfoPtr == nil {
		return r
	}
	r = *o.VserverInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VserverGetResponseResultAttributes) setValues(newValue VserverInfoType) *VserverGetResponseResultAttributes {
	o.VserverInfoPtr = &newValue
	return o
}

// Attributes is a 'getter' method
func (o *VserverGetResponseResult) Attributes() VserverGetResponseResultAttributes {
	var r VserverGetResponseResultAttributes
	if o.AttributesPtr == nil {
		return r
	}
	r = *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VserverGetResponseResult) SetAttributes(newValue VserverGetResponseResultAttributes) *VserverGetResponseResult {
	o.AttributesPtr = &newValue
	return o
}
