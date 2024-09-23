// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeSubsystemDeleteRequest is a structure to represent a nvme-subsystem-delete Request ZAPI object
type NvmeSubsystemDeleteRequest struct {
	XMLName            xml.Name `xml:"nvme-subsystem-delete"`
	SkipHostCheckPtr   *bool    `xml:"skip-host-check"`
	SkipMappedCheckPtr *bool    `xml:"skip-mapped-check"`
	SubsystemPtr       *string  `xml:"subsystem"`
}

// NvmeSubsystemDeleteResponse is a structure to represent a nvme-subsystem-delete Response ZAPI object
type NvmeSubsystemDeleteResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeSubsystemDeleteResponseResult `xml:"results"`
}

// NewNvmeSubsystemDeleteResponse is a factory method for creating new instances of NvmeSubsystemDeleteResponse objects
func NewNvmeSubsystemDeleteResponse() *NvmeSubsystemDeleteResponse {
	return &NvmeSubsystemDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemDeleteResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemDeleteResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemDeleteResponseResult is a structure to represent a nvme-subsystem-delete Response Result ZAPI object
type NvmeSubsystemDeleteResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemDeleteRequest is a factory method for creating new instances of NvmeSubsystemDeleteRequest objects
func NewNvmeSubsystemDeleteRequest() *NvmeSubsystemDeleteRequest {
	return &NvmeSubsystemDeleteRequest{}
}

// NewNvmeSubsystemDeleteResponseResult is a factory method for creating new instances of NvmeSubsystemDeleteResponseResult objects
func NewNvmeSubsystemDeleteResponseResult() *NvmeSubsystemDeleteResponseResult {
	return &NvmeSubsystemDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemDeleteRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemDeleteResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemDeleteRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemDeleteResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemDeleteResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemDeleteResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemDeleteRequest", NewNvmeSubsystemDeleteResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemDeleteResponse), err
}

// SkipHostCheck is a 'getter' method
func (o *NvmeSubsystemDeleteRequest) SkipHostCheck() bool {
	var r bool
	if o.SkipHostCheckPtr == nil {
		return r
	}
	r = *o.SkipHostCheckPtr
	return r
}

// SetSkipHostCheck is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemDeleteRequest) SetSkipHostCheck(newValue bool) *NvmeSubsystemDeleteRequest {
	o.SkipHostCheckPtr = &newValue
	return o
}

// SkipMappedCheck is a 'getter' method
func (o *NvmeSubsystemDeleteRequest) SkipMappedCheck() bool {
	var r bool
	if o.SkipMappedCheckPtr == nil {
		return r
	}
	r = *o.SkipMappedCheckPtr
	return r
}

// SetSkipMappedCheck is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemDeleteRequest) SetSkipMappedCheck(newValue bool) *NvmeSubsystemDeleteRequest {
	o.SkipMappedCheckPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemDeleteRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemDeleteRequest) SetSubsystem(newValue string) *NvmeSubsystemDeleteRequest {
	o.SubsystemPtr = &newValue
	return o
}
