// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemMapRemoveRequest is a structure to represent a nvme-subsystem-map-remove Request ZAPI object
type NvmeSubsystemMapRemoveRequest struct {
	XMLName      xml.Name     `xml:"nvme-subsystem-map-remove"`
	PathPtr      *LunPathType `xml:"path"`
	SubsystemPtr *string      `xml:"subsystem"`
}

// NvmeSubsystemMapRemoveResponse is a structure to represent a nvme-subsystem-map-remove Response ZAPI object
type NvmeSubsystemMapRemoveResponse struct {
	XMLName         xml.Name                             `xml:"netapp"`
	ResponseVersion string                               `xml:"version,attr"`
	ResponseXmlns   string                               `xml:"xmlns,attr"`
	Result          NvmeSubsystemMapRemoveResponseResult `xml:"results"`
}

// NewNvmeSubsystemMapRemoveResponse is a factory method for creating new instances of NvmeSubsystemMapRemoveResponse objects
func NewNvmeSubsystemMapRemoveResponse() *NvmeSubsystemMapRemoveResponse {
	return &NvmeSubsystemMapRemoveResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapRemoveResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapRemoveResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemMapRemoveResponseResult is a structure to represent a nvme-subsystem-map-remove Response Result ZAPI object
type NvmeSubsystemMapRemoveResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemMapRemoveRequest is a factory method for creating new instances of NvmeSubsystemMapRemoveRequest objects
func NewNvmeSubsystemMapRemoveRequest() *NvmeSubsystemMapRemoveRequest {
	return &NvmeSubsystemMapRemoveRequest{}
}

// NewNvmeSubsystemMapRemoveResponseResult is a factory method for creating new instances of NvmeSubsystemMapRemoveResponseResult objects
func NewNvmeSubsystemMapRemoveResponseResult() *NvmeSubsystemMapRemoveResponseResult {
	return &NvmeSubsystemMapRemoveResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapRemoveRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapRemoveResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapRemoveRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapRemoveResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapRemoveRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemMapRemoveResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapRemoveRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemMapRemoveResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemMapRemoveRequest", NewNvmeSubsystemMapRemoveResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemMapRemoveResponse), err
}

// Path is a 'getter' method
func (o *NvmeSubsystemMapRemoveRequest) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapRemoveRequest) SetPath(newValue LunPathType) *NvmeSubsystemMapRemoveRequest {
	o.PathPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemMapRemoveRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapRemoveRequest) SetSubsystem(newValue string) *NvmeSubsystemMapRemoveRequest {
	o.SubsystemPtr = &newValue
	return o
}
