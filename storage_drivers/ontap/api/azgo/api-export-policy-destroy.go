// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportPolicyDestroyRequest is a structure to represent a export-policy-destroy Request ZAPI object
type ExportPolicyDestroyRequest struct {
	XMLName       xml.Name              `xml:"export-policy-destroy"`
	PolicyNamePtr *ExportPolicyNameType `xml:"policy-name"`
}

// ExportPolicyDestroyResponse is a structure to represent a export-policy-destroy Response ZAPI object
type ExportPolicyDestroyResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          ExportPolicyDestroyResponseResult `xml:"results"`
}

// NewExportPolicyDestroyResponse is a factory method for creating new instances of ExportPolicyDestroyResponse objects
func NewExportPolicyDestroyResponse() *ExportPolicyDestroyResponse {
	return &ExportPolicyDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportPolicyDestroyResponseResult is a structure to represent a export-policy-destroy Response Result ZAPI object
type ExportPolicyDestroyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewExportPolicyDestroyRequest is a factory method for creating new instances of ExportPolicyDestroyRequest objects
func NewExportPolicyDestroyRequest() *ExportPolicyDestroyRequest {
	return &ExportPolicyDestroyRequest{}
}

// NewExportPolicyDestroyResponseResult is a factory method for creating new instances of ExportPolicyDestroyResponseResult objects
func NewExportPolicyDestroyResponseResult() *ExportPolicyDestroyResponseResult {
	return &ExportPolicyDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportPolicyDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*ExportPolicyDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportPolicyDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportPolicyDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportPolicyDestroyRequest", NewExportPolicyDestroyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportPolicyDestroyResponse), err
}

// PolicyName is a 'getter' method
func (o *ExportPolicyDestroyRequest) PolicyName() ExportPolicyNameType {
	var r ExportPolicyNameType
	if o.PolicyNamePtr == nil {
		return r
	}
	r = *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportPolicyDestroyRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportPolicyDestroyRequest {
	o.PolicyNamePtr = &newValue
	return o
}
