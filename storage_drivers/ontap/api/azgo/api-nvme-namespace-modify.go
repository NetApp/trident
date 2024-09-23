// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeNamespaceModifyRequest is a structure to represent a nvme-namespace-modify Request ZAPI object
type NvmeNamespaceModifyRequest struct {
	XMLName    xml.Name     `xml:"nvme-namespace-modify"`
	CommentPtr *string      `xml:"comment"`
	PathPtr    *LunPathType `xml:"path"`
}

// NvmeNamespaceModifyResponse is a structure to represent a nvme-namespace-modify Response ZAPI object
type NvmeNamespaceModifyResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeNamespaceModifyResponseResult `xml:"results"`
}

// NewNvmeNamespaceModifyResponse is a factory method for creating new instances of NvmeNamespaceModifyResponse objects
func NewNvmeNamespaceModifyResponse() *NvmeNamespaceModifyResponse {
	return &NvmeNamespaceModifyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceModifyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceModifyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeNamespaceModifyResponseResult is a structure to represent a nvme-namespace-modify Response Result ZAPI object
type NvmeNamespaceModifyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeNamespaceModifyRequest is a factory method for creating new instances of NvmeNamespaceModifyRequest objects
func NewNvmeNamespaceModifyRequest() *NvmeNamespaceModifyRequest {
	return &NvmeNamespaceModifyRequest{}
}

// NewNvmeNamespaceModifyResponseResult is a factory method for creating new instances of NvmeNamespaceModifyResponseResult objects
func NewNvmeNamespaceModifyResponseResult() *NvmeNamespaceModifyResponseResult {
	return &NvmeNamespaceModifyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceModifyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceModifyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceModifyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceModifyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceModifyRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeNamespaceModifyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceModifyRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeNamespaceModifyResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeNamespaceModifyRequest", NewNvmeNamespaceModifyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeNamespaceModifyResponse), err
}

// Comment is a 'getter' method
func (o *NvmeNamespaceModifyRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceModifyRequest) SetComment(newValue string) *NvmeNamespaceModifyRequest {
	o.CommentPtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *NvmeNamespaceModifyRequest) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceModifyRequest) SetPath(newValue LunPathType) *NvmeNamespaceModifyRequest {
	o.PathPtr = &newValue
	return o
}
