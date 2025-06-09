// Code generated automatically. DO NOT EDIT.
// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// CgCommitRequest is a structure to represent a cg-commit Request ZAPI object
type CgCommitRequest struct {
	XMLName xml.Name `xml:"cg-commit"`
	CgIdPtr *int     `xml:"cg-id"`
}

// CgCommitResponse is a structure to represent a cg-commit Response ZAPI object
type CgCommitResponse struct {
	XMLName         xml.Name               `xml:"netapp"`
	ResponseVersion string                 `xml:"version,attr"`
	ResponseXmlns   string                 `xml:"xmlns,attr"`
	Result          CgCommitResponseResult `xml:"results"`
}

// NewCgCommitResponse is a factory method for creating new instances of CgCommitResponse objects
func NewCgCommitResponse() *CgCommitResponse {
	return &CgCommitResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgCommitResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CgCommitResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CgCommitResponseResult is a structure to represent a cg-commit Response Result ZAPI object
type CgCommitResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCgCommitRequest is a factory method for creating new instances of CgCommitRequest objects
func NewCgCommitRequest() *CgCommitRequest {
	return &CgCommitRequest{}
}

// NewCgCommitResponseResult is a factory method for creating new instances of CgCommitResponseResult objects
func NewCgCommitResponseResult() *CgCommitResponseResult {
	return &CgCommitResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CgCommitRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CgCommitResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgCommitRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgCommitResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CgCommitRequest) ExecuteUsing(zr *ZapiRunner) (*CgCommitResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CgCommitRequest) executeWithoutIteration(zr *ZapiRunner) (*CgCommitResponse, error) {
	result, err := zr.ExecuteUsing(o, "CgCommitRequest", NewCgCommitResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CgCommitResponse), err
}

// CgId is a 'getter' method
func (o *CgCommitRequest) CgId() int {
	var r int
	if o.CgIdPtr == nil {
		return r
	}
	r = *o.CgIdPtr
	return r
}

// SetCgId is a fluent style 'setter' method that can be chained
func (o *CgCommitRequest) SetCgId(newValue int) *CgCommitRequest {
	o.CgIdPtr = &newValue
	return o
}
