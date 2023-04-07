// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeRevertAnagrpidsRequest is a structure to represent a nvme-revert-anagrpids Request ZAPI object
type NvmeRevertAnagrpidsRequest struct {
	XMLName    xml.Name         `xml:"nvme-revert-anagrpids"`
	VserverPtr *VserverNameType `xml:"vserver"`
}

// NvmeRevertAnagrpidsResponse is a structure to represent a nvme-revert-anagrpids Response ZAPI object
type NvmeRevertAnagrpidsResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeRevertAnagrpidsResponseResult `xml:"results"`
}

// NewNvmeRevertAnagrpidsResponse is a factory method for creating new instances of NvmeRevertAnagrpidsResponse objects
func NewNvmeRevertAnagrpidsResponse() *NvmeRevertAnagrpidsResponse {
	return &NvmeRevertAnagrpidsResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeRevertAnagrpidsResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeRevertAnagrpidsResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeRevertAnagrpidsResponseResult is a structure to represent a nvme-revert-anagrpids Response Result ZAPI object
type NvmeRevertAnagrpidsResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeRevertAnagrpidsRequest is a factory method for creating new instances of NvmeRevertAnagrpidsRequest objects
func NewNvmeRevertAnagrpidsRequest() *NvmeRevertAnagrpidsRequest {
	return &NvmeRevertAnagrpidsRequest{}
}

// NewNvmeRevertAnagrpidsResponseResult is a factory method for creating new instances of NvmeRevertAnagrpidsResponseResult objects
func NewNvmeRevertAnagrpidsResponseResult() *NvmeRevertAnagrpidsResponseResult {
	return &NvmeRevertAnagrpidsResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeRevertAnagrpidsRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeRevertAnagrpidsResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeRevertAnagrpidsRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeRevertAnagrpidsResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeRevertAnagrpidsRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeRevertAnagrpidsResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeRevertAnagrpidsRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeRevertAnagrpidsResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeRevertAnagrpidsRequest", NewNvmeRevertAnagrpidsResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeRevertAnagrpidsResponse), err
}

// Vserver is a 'getter' method
func (o *NvmeRevertAnagrpidsRequest) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeRevertAnagrpidsRequest) SetVserver(newValue VserverNameType) *NvmeRevertAnagrpidsRequest {
	o.VserverPtr = &newValue
	return o
}
