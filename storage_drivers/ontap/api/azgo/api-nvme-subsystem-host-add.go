// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemHostAddRequest is a structure to represent a nvme-subsystem-host-add Request ZAPI object
type NvmeSubsystemHostAddRequest struct {
	XMLName      xml.Name `xml:"nvme-subsystem-host-add"`
	HostNqnPtr   *string  `xml:"host-nqn"`
	SubsystemPtr *string  `xml:"subsystem"`
}

// NvmeSubsystemHostAddResponse is a structure to represent a nvme-subsystem-host-add Response ZAPI object
type NvmeSubsystemHostAddResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          NvmeSubsystemHostAddResponseResult `xml:"results"`
}

// NewNvmeSubsystemHostAddResponse is a factory method for creating new instances of NvmeSubsystemHostAddResponse objects
func NewNvmeSubsystemHostAddResponse() *NvmeSubsystemHostAddResponse {
	return &NvmeSubsystemHostAddResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostAddResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostAddResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemHostAddResponseResult is a structure to represent a nvme-subsystem-host-add Response Result ZAPI object
type NvmeSubsystemHostAddResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemHostAddRequest is a factory method for creating new instances of NvmeSubsystemHostAddRequest objects
func NewNvmeSubsystemHostAddRequest() *NvmeSubsystemHostAddRequest {
	return &NvmeSubsystemHostAddRequest{}
}

// NewNvmeSubsystemHostAddResponseResult is a factory method for creating new instances of NvmeSubsystemHostAddResponseResult objects
func NewNvmeSubsystemHostAddResponseResult() *NvmeSubsystemHostAddResponseResult {
	return &NvmeSubsystemHostAddResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostAddRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostAddResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostAddRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostAddResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostAddRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemHostAddResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostAddRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemHostAddResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemHostAddRequest", NewNvmeSubsystemHostAddResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemHostAddResponse), err
}

// HostNqn is a 'getter' method
func (o *NvmeSubsystemHostAddRequest) HostNqn() string {
	var r string
	if o.HostNqnPtr == nil {
		return r
	}
	r = *o.HostNqnPtr
	return r
}

// SetHostNqn is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostAddRequest) SetHostNqn(newValue string) *NvmeSubsystemHostAddRequest {
	o.HostNqnPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemHostAddRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostAddRequest) SetSubsystem(newValue string) *NvmeSubsystemHostAddRequest {
	o.SubsystemPtr = &newValue
	return o
}
