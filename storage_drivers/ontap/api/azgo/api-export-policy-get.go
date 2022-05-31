// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// ExportPolicyGetRequest is a structure to represent a export-policy-get Request ZAPI object
type ExportPolicyGetRequest struct {
	XMLName              xml.Name                                 `xml:"export-policy-get"`
	DesiredAttributesPtr *ExportPolicyGetRequestDesiredAttributes `xml:"desired-attributes"`
	PolicyNamePtr        *ExportPolicyNameType                    `xml:"policy-name"`
}

// ExportPolicyGetResponse is a structure to represent a export-policy-get Response ZAPI object
type ExportPolicyGetResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          ExportPolicyGetResponseResult `xml:"results"`
}

// NewExportPolicyGetResponse is a factory method for creating new instances of ExportPolicyGetResponse objects
func NewExportPolicyGetResponse() *ExportPolicyGetResponse {
	return &ExportPolicyGetResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyGetResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyGetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportPolicyGetResponseResult is a structure to represent a export-policy-get Response Result ZAPI object
type ExportPolicyGetResponseResult struct {
	XMLName          xml.Name                                 `xml:"results"`
	ResultStatusAttr string                                   `xml:"status,attr"`
	ResultReasonAttr string                                   `xml:"reason,attr"`
	ResultErrnoAttr  string                                   `xml:"errno,attr"`
	AttributesPtr    *ExportPolicyGetResponseResultAttributes `xml:"attributes"`
}

// NewExportPolicyGetRequest is a factory method for creating new instances of ExportPolicyGetRequest objects
func NewExportPolicyGetRequest() *ExportPolicyGetRequest {
	return &ExportPolicyGetRequest{}
}

// NewExportPolicyGetResponseResult is a factory method for creating new instances of ExportPolicyGetResponseResult objects
func NewExportPolicyGetResponseResult() *ExportPolicyGetResponseResult {
	return &ExportPolicyGetResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyGetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyGetResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyGetRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyGetResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportPolicyGetRequest) ExecuteUsing(zr *ZapiRunner) (*ExportPolicyGetResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportPolicyGetRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportPolicyGetResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportPolicyGetRequest", NewExportPolicyGetResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportPolicyGetResponse), err
}

// ExportPolicyGetRequestDesiredAttributes is a wrapper
type ExportPolicyGetRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	ExportPolicyInfoPtr *ExportPolicyInfoType `xml:"export-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyGetRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportPolicyInfo is a 'getter' method
func (o *ExportPolicyGetRequestDesiredAttributes) ExportPolicyInfo() ExportPolicyInfoType {
	var r ExportPolicyInfoType
	if o.ExportPolicyInfoPtr == nil {
		return r
	}
	r = *o.ExportPolicyInfoPtr
	return r
}

// SetExportPolicyInfo is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetRequestDesiredAttributes) SetExportPolicyInfo(newValue ExportPolicyInfoType) *ExportPolicyGetRequestDesiredAttributes {
	o.ExportPolicyInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *ExportPolicyGetRequest) DesiredAttributes() ExportPolicyGetRequestDesiredAttributes {
	var r ExportPolicyGetRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetRequest) SetDesiredAttributes(newValue ExportPolicyGetRequestDesiredAttributes) *ExportPolicyGetRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// PolicyName is a 'getter' method
func (o *ExportPolicyGetRequest) PolicyName() ExportPolicyNameType {
	var r ExportPolicyNameType
	if o.PolicyNamePtr == nil {
		return r
	}
	r = *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportPolicyGetRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// ExportPolicyGetResponseResultAttributes is a wrapper
type ExportPolicyGetResponseResultAttributes struct {
	XMLName             xml.Name              `xml:"attributes"`
	ExportPolicyInfoPtr *ExportPolicyInfoType `xml:"export-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyGetResponseResultAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportPolicyInfo is a 'getter' method
func (o *ExportPolicyGetResponseResultAttributes) ExportPolicyInfo() ExportPolicyInfoType {
	var r ExportPolicyInfoType
	if o.ExportPolicyInfoPtr == nil {
		return r
	}
	r = *o.ExportPolicyInfoPtr
	return r
}

// SetExportPolicyInfo is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetResponseResultAttributes) SetExportPolicyInfo(newValue ExportPolicyInfoType) *ExportPolicyGetResponseResultAttributes {
	o.ExportPolicyInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *ExportPolicyGetResponseResultAttributes) values() ExportPolicyInfoType {
	var r ExportPolicyInfoType
	if o.ExportPolicyInfoPtr == nil {
		return r
	}
	r = *o.ExportPolicyInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetResponseResultAttributes) setValues(newValue ExportPolicyInfoType) *ExportPolicyGetResponseResultAttributes {
	o.ExportPolicyInfoPtr = &newValue
	return o
}

// Attributes is a 'getter' method
func (o *ExportPolicyGetResponseResult) Attributes() ExportPolicyGetResponseResultAttributes {
	var r ExportPolicyGetResponseResultAttributes
	if o.AttributesPtr == nil {
		return r
	}
	r = *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *ExportPolicyGetResponseResult) SetAttributes(newValue ExportPolicyGetResponseResultAttributes) *ExportPolicyGetResponseResult {
	o.AttributesPtr = &newValue
	return o
}
