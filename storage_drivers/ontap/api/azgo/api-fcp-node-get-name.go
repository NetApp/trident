// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// FcpNodeGetNameRequest is a structure to represent a fcp-node-get-name Request ZAPI object
type FcpNodeGetNameRequest struct {
	XMLName xml.Name `xml:"fcp-node-get-name"`
}

// FcpNodeGetNameResponse is a structure to represent a fcp-node-get-name Response ZAPI object
type FcpNodeGetNameResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          IscsiNodeGetNameResponseResult `xml:"results"`
}

// NewFcpNodeGetNameResponse is a factory method for creating new instances of FcpNodeGetNameResponse objects
func NewFcpNodeGetNameResponse() *FcpNodeGetNameResponse {
	return &FcpNodeGetNameResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpNodeGetNameResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *FcpNodeGetNameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// FcpNodeGetNameResponseResult is a structure to represent a fcp-node-get-name Response Result ZAPI object
type FcpNodeGetNameResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	NodeNamePtr      *string  `xml:"node-name"`
}

// NewFcpNodeGetNameRequest is a factory method for creating new instances of FcpNodeGetNameRequest objects
func NewFcpNodeGetNameRequest() *FcpNodeGetNameRequest {
	return &FcpNodeGetNameRequest{}
}

// NewFcpNodeGetNameResponseResult is a factory method for creating new instances of FcpNodeGetNameResponseResult objects
func NewFcpNodeGetNameResponseResult() *FcpNodeGetNameResponseResult {
	return &FcpNodeGetNameResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *FcpNodeGetNameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *FcpNodeGetNameResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpNodeGetNameRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpNodeGetNameResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpNodeGetNameRequest) ExecuteUsing(zr *ZapiRunner) (*FcpNodeGetNameResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpNodeGetNameRequest) executeWithoutIteration(zr *ZapiRunner) (*FcpNodeGetNameResponse, error) {
	result, err := zr.ExecuteUsing(o, "FcpNodeGetNameRequest", NewFcpNodeGetNameResponse())
	if result == nil {
		return nil, err
	}
	return result.(*FcpNodeGetNameResponse), err
}

// NodeName is a 'getter' method
func (o *FcpNodeGetNameResponseResult) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *FcpNodeGetNameResponseResult) SetNodeName(newValue string) *FcpNodeGetNameResponseResult {
	o.NodeNamePtr = &newValue
	return o
}
