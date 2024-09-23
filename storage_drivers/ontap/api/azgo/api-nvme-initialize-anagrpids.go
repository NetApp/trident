// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeInitializeAnagrpidsRequest is a structure to represent a nvme-initialize-anagrpids Request ZAPI object
type NvmeInitializeAnagrpidsRequest struct {
	XMLName    xml.Name         `xml:"nvme-initialize-anagrpids"`
	VserverPtr *VserverNameType `xml:"vserver"`
}

// NvmeInitializeAnagrpidsResponse is a structure to represent a nvme-initialize-anagrpids Response ZAPI object
type NvmeInitializeAnagrpidsResponse struct {
	XMLName         xml.Name                              `xml:"netapp"`
	ResponseVersion string                                `xml:"version,attr"`
	ResponseXmlns   string                                `xml:"xmlns,attr"`
	Result          NvmeInitializeAnagrpidsResponseResult `xml:"results"`
}

// NewNvmeInitializeAnagrpidsResponse is a factory method for creating new instances of NvmeInitializeAnagrpidsResponse objects
func NewNvmeInitializeAnagrpidsResponse() *NvmeInitializeAnagrpidsResponse {
	return &NvmeInitializeAnagrpidsResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInitializeAnagrpidsResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeInitializeAnagrpidsResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeInitializeAnagrpidsResponseResult is a structure to represent a nvme-initialize-anagrpids Response Result ZAPI object
type NvmeInitializeAnagrpidsResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeInitializeAnagrpidsRequest is a factory method for creating new instances of NvmeInitializeAnagrpidsRequest objects
func NewNvmeInitializeAnagrpidsRequest() *NvmeInitializeAnagrpidsRequest {
	return &NvmeInitializeAnagrpidsRequest{}
}

// NewNvmeInitializeAnagrpidsResponseResult is a factory method for creating new instances of NvmeInitializeAnagrpidsResponseResult objects
func NewNvmeInitializeAnagrpidsResponseResult() *NvmeInitializeAnagrpidsResponseResult {
	return &NvmeInitializeAnagrpidsResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeInitializeAnagrpidsRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeInitializeAnagrpidsResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInitializeAnagrpidsRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInitializeAnagrpidsResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeInitializeAnagrpidsRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeInitializeAnagrpidsResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeInitializeAnagrpidsRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeInitializeAnagrpidsResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeInitializeAnagrpidsRequest", NewNvmeInitializeAnagrpidsResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeInitializeAnagrpidsResponse), err
}

// Vserver is a 'getter' method
func (o *NvmeInitializeAnagrpidsRequest) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeInitializeAnagrpidsRequest) SetVserver(newValue VserverNameType) *NvmeInitializeAnagrpidsRequest {
	o.VserverPtr = &newValue
	return o
}
