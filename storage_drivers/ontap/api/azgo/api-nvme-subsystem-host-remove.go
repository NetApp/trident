// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeSubsystemHostRemoveRequest is a structure to represent a nvme-subsystem-host-remove Request ZAPI object
type NvmeSubsystemHostRemoveRequest struct {
	XMLName      xml.Name `xml:"nvme-subsystem-host-remove"`
	HostNqnPtr   *string  `xml:"host-nqn"`
	SubsystemPtr *string  `xml:"subsystem"`
}

// NvmeSubsystemHostRemoveResponse is a structure to represent a nvme-subsystem-host-remove Response ZAPI object
type NvmeSubsystemHostRemoveResponse struct {
	XMLName         xml.Name                              `xml:"netapp"`
	ResponseVersion string                                `xml:"version,attr"`
	ResponseXmlns   string                                `xml:"xmlns,attr"`
	Result          NvmeSubsystemHostRemoveResponseResult `xml:"results"`
}

// NewNvmeSubsystemHostRemoveResponse is a factory method for creating new instances of NvmeSubsystemHostRemoveResponse objects
func NewNvmeSubsystemHostRemoveResponse() *NvmeSubsystemHostRemoveResponse {
	return &NvmeSubsystemHostRemoveResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostRemoveResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostRemoveResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemHostRemoveResponseResult is a structure to represent a nvme-subsystem-host-remove Response Result ZAPI object
type NvmeSubsystemHostRemoveResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemHostRemoveRequest is a factory method for creating new instances of NvmeSubsystemHostRemoveRequest objects
func NewNvmeSubsystemHostRemoveRequest() *NvmeSubsystemHostRemoveRequest {
	return &NvmeSubsystemHostRemoveRequest{}
}

// NewNvmeSubsystemHostRemoveResponseResult is a factory method for creating new instances of NvmeSubsystemHostRemoveResponseResult objects
func NewNvmeSubsystemHostRemoveResponseResult() *NvmeSubsystemHostRemoveResponseResult {
	return &NvmeSubsystemHostRemoveResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostRemoveRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostRemoveResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostRemoveRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostRemoveResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostRemoveRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemHostRemoveResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostRemoveRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemHostRemoveResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemHostRemoveRequest", NewNvmeSubsystemHostRemoveResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemHostRemoveResponse), err
}

// HostNqn is a 'getter' method
func (o *NvmeSubsystemHostRemoveRequest) HostNqn() string {
	var r string
	if o.HostNqnPtr == nil {
		return r
	}
	r = *o.HostNqnPtr
	return r
}

// SetHostNqn is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostRemoveRequest) SetHostNqn(newValue string) *NvmeSubsystemHostRemoveRequest {
	o.HostNqnPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemHostRemoveRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostRemoveRequest) SetSubsystem(newValue string) *NvmeSubsystemHostRemoveRequest {
	o.SubsystemPtr = &newValue
	return o
}
