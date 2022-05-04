// Code generated automatically. DO NOT EDIT.
// Copyright 2020 NetApp, Inc. All Rights Reserved.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportRuleDestroyRequest is a structure to represent a export-rule-destroy Request ZAPI object
type ExportRuleDestroyRequest struct {
	XMLName       xml.Name              `xml:"export-rule-destroy"`
	PolicyNamePtr *ExportPolicyNameType `xml:"policy-name"`
	RuleIndexPtr  *int                  `xml:"rule-index"`
}

// ExportRuleDestroyResponse is a structure to represent a export-rule-destroy Response ZAPI object
type ExportRuleDestroyResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          ExportRuleDestroyResponseResult `xml:"results"`
}

// NewExportRuleDestroyResponse is a factory method for creating new instances of ExportRuleDestroyResponse objects
func NewExportRuleDestroyResponse() *ExportRuleDestroyResponse {
	return &ExportRuleDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportRuleDestroyResponseResult is a structure to represent a export-rule-destroy Response Result ZAPI object
type ExportRuleDestroyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewExportRuleDestroyRequest is a factory method for creating new instances of ExportRuleDestroyRequest objects
func NewExportRuleDestroyRequest() *ExportRuleDestroyRequest {
	return &ExportRuleDestroyRequest{}
}

// NewExportRuleDestroyResponseResult is a factory method for creating new instances of ExportRuleDestroyResponseResult objects
func NewExportRuleDestroyResponseResult() *ExportRuleDestroyResponseResult {
	return &ExportRuleDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*ExportRuleDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportRuleDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportRuleDestroyRequest", NewExportRuleDestroyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportRuleDestroyResponse), err
}

// PolicyName is a 'getter' method
func (o *ExportRuleDestroyRequest) PolicyName() ExportPolicyNameType {
	r := *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportRuleDestroyRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportRuleDestroyRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// RuleIndex is a 'getter' method
func (o *ExportRuleDestroyRequest) RuleIndex() int {
	r := *o.RuleIndexPtr
	return r
}

// SetRuleIndex is a fluent style 'setter' method that can be chained
func (o *ExportRuleDestroyRequest) SetRuleIndex(newValue int) *ExportRuleDestroyRequest {
	o.RuleIndexPtr = &newValue
	return o
}
