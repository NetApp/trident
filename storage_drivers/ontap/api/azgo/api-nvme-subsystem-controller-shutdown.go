// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeSubsystemControllerShutdownRequest is a structure to represent a nvme-subsystem-controller-shutdown Request ZAPI object
type NvmeSubsystemControllerShutdownRequest struct {
	XMLName         xml.Name `xml:"nvme-subsystem-controller-shutdown"`
	ControllerIdPtr *int     `xml:"controller-id"`
	SubsystemPtr    *string  `xml:"subsystem"`
}

// NvmeSubsystemControllerShutdownResponse is a structure to represent a nvme-subsystem-controller-shutdown Response ZAPI object
type NvmeSubsystemControllerShutdownResponse struct {
	XMLName         xml.Name                                      `xml:"netapp"`
	ResponseVersion string                                        `xml:"version,attr"`
	ResponseXmlns   string                                        `xml:"xmlns,attr"`
	Result          NvmeSubsystemControllerShutdownResponseResult `xml:"results"`
}

// NewNvmeSubsystemControllerShutdownResponse is a factory method for creating new instances of NvmeSubsystemControllerShutdownResponse objects
func NewNvmeSubsystemControllerShutdownResponse() *NvmeSubsystemControllerShutdownResponse {
	return &NvmeSubsystemControllerShutdownResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerShutdownResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerShutdownResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemControllerShutdownResponseResult is a structure to represent a nvme-subsystem-controller-shutdown Response Result ZAPI object
type NvmeSubsystemControllerShutdownResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemControllerShutdownRequest is a factory method for creating new instances of NvmeSubsystemControllerShutdownRequest objects
func NewNvmeSubsystemControllerShutdownRequest() *NvmeSubsystemControllerShutdownRequest {
	return &NvmeSubsystemControllerShutdownRequest{}
}

// NewNvmeSubsystemControllerShutdownResponseResult is a factory method for creating new instances of NvmeSubsystemControllerShutdownResponseResult objects
func NewNvmeSubsystemControllerShutdownResponseResult() *NvmeSubsystemControllerShutdownResponseResult {
	return &NvmeSubsystemControllerShutdownResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerShutdownRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerShutdownResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerShutdownRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerShutdownResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemControllerShutdownRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemControllerShutdownResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemControllerShutdownRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemControllerShutdownResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemControllerShutdownRequest", NewNvmeSubsystemControllerShutdownResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemControllerShutdownResponse), err
}

// ControllerId is a 'getter' method
func (o *NvmeSubsystemControllerShutdownRequest) ControllerId() int {
	var r int
	if o.ControllerIdPtr == nil {
		return r
	}
	r = *o.ControllerIdPtr
	return r
}

// SetControllerId is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerShutdownRequest) SetControllerId(newValue int) *NvmeSubsystemControllerShutdownRequest {
	o.ControllerIdPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemControllerShutdownRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerShutdownRequest) SetSubsystem(newValue string) *NvmeSubsystemControllerShutdownRequest {
	o.SubsystemPtr = &newValue
	return o
}
