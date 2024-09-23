// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeDeleteRequest is a structure to represent a nvme-delete Request ZAPI object
type NvmeDeleteRequest struct {
	XMLName xml.Name `xml:"nvme-delete"`
}

// NvmeDeleteResponse is a structure to represent a nvme-delete Response ZAPI object
type NvmeDeleteResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          NvmeDeleteResponseResult `xml:"results"`
}

// NewNvmeDeleteResponse is a factory method for creating new instances of NvmeDeleteResponse objects
func NewNvmeDeleteResponse() *NvmeDeleteResponse {
	return &NvmeDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeDeleteResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeDeleteResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeDeleteResponseResult is a structure to represent a nvme-delete Response Result ZAPI object
type NvmeDeleteResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeDeleteRequest is a factory method for creating new instances of NvmeDeleteRequest objects
func NewNvmeDeleteRequest() *NvmeDeleteRequest {
	return &NvmeDeleteRequest{}
}

// NewNvmeDeleteResponseResult is a factory method for creating new instances of NvmeDeleteResponseResult objects
func NewNvmeDeleteResponseResult() *NvmeDeleteResponseResult {
	return &NvmeDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeDeleteRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeDeleteResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeDeleteRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeDeleteResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeDeleteResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeDeleteResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeDeleteRequest", NewNvmeDeleteResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeDeleteResponse), err
}
