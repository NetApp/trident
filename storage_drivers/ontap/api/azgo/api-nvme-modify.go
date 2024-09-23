// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeModifyRequest is a structure to represent a nvme-modify Request ZAPI object
type NvmeModifyRequest struct {
	XMLName        xml.Name `xml:"nvme-modify"`
	IsAvailablePtr *bool    `xml:"is-available"`
}

// NvmeModifyResponse is a structure to represent a nvme-modify Response ZAPI object
type NvmeModifyResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          NvmeModifyResponseResult `xml:"results"`
}

// NewNvmeModifyResponse is a factory method for creating new instances of NvmeModifyResponse objects
func NewNvmeModifyResponse() *NvmeModifyResponse {
	return &NvmeModifyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeModifyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeModifyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeModifyResponseResult is a structure to represent a nvme-modify Response Result ZAPI object
type NvmeModifyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeModifyRequest is a factory method for creating new instances of NvmeModifyRequest objects
func NewNvmeModifyRequest() *NvmeModifyRequest {
	return &NvmeModifyRequest{}
}

// NewNvmeModifyResponseResult is a factory method for creating new instances of NvmeModifyResponseResult objects
func NewNvmeModifyResponseResult() *NvmeModifyResponseResult {
	return &NvmeModifyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeModifyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeModifyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeModifyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeModifyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeModifyRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeModifyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeModifyRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeModifyResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeModifyRequest", NewNvmeModifyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeModifyResponse), err
}

// IsAvailable is a 'getter' method
func (o *NvmeModifyRequest) IsAvailable() bool {
	var r bool
	if o.IsAvailablePtr == nil {
		return r
	}
	r = *o.IsAvailablePtr
	return r
}

// SetIsAvailable is a fluent style 'setter' method that can be chained
func (o *NvmeModifyRequest) SetIsAvailable(newValue bool) *NvmeModifyRequest {
	o.IsAvailablePtr = &newValue
	return o
}
