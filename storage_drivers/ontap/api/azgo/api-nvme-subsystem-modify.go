// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemModifyRequest is a structure to represent a nvme-subsystem-modify Request ZAPI object
type NvmeSubsystemModifyRequest struct {
	XMLName          xml.Name `xml:"nvme-subsystem-modify"`
	CommentPtr       *string  `xml:"comment"`
	DeleteOnUnmapPtr *bool    `xml:"delete-on-unmap"`
	SubsystemPtr     *string  `xml:"subsystem"`
}

// NvmeSubsystemModifyResponse is a structure to represent a nvme-subsystem-modify Response ZAPI object
type NvmeSubsystemModifyResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeSubsystemModifyResponseResult `xml:"results"`
}

// NewNvmeSubsystemModifyResponse is a factory method for creating new instances of NvmeSubsystemModifyResponse objects
func NewNvmeSubsystemModifyResponse() *NvmeSubsystemModifyResponse {
	return &NvmeSubsystemModifyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemModifyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemModifyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemModifyResponseResult is a structure to represent a nvme-subsystem-modify Response Result ZAPI object
type NvmeSubsystemModifyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeSubsystemModifyRequest is a factory method for creating new instances of NvmeSubsystemModifyRequest objects
func NewNvmeSubsystemModifyRequest() *NvmeSubsystemModifyRequest {
	return &NvmeSubsystemModifyRequest{}
}

// NewNvmeSubsystemModifyResponseResult is a factory method for creating new instances of NvmeSubsystemModifyResponseResult objects
func NewNvmeSubsystemModifyResponseResult() *NvmeSubsystemModifyResponseResult {
	return &NvmeSubsystemModifyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemModifyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemModifyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemModifyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemModifyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemModifyRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemModifyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemModifyRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemModifyResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemModifyRequest", NewNvmeSubsystemModifyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemModifyResponse), err
}

// Comment is a 'getter' method
func (o *NvmeSubsystemModifyRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemModifyRequest) SetComment(newValue string) *NvmeSubsystemModifyRequest {
	o.CommentPtr = &newValue
	return o
}

// DeleteOnUnmap is a 'getter' method
func (o *NvmeSubsystemModifyRequest) DeleteOnUnmap() bool {
	var r bool
	if o.DeleteOnUnmapPtr == nil {
		return r
	}
	r = *o.DeleteOnUnmapPtr
	return r
}

// SetDeleteOnUnmap is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemModifyRequest) SetDeleteOnUnmap(newValue bool) *NvmeSubsystemModifyRequest {
	o.DeleteOnUnmapPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemModifyRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemModifyRequest) SetSubsystem(newValue string) *NvmeSubsystemModifyRequest {
	o.SubsystemPtr = &newValue
	return o
}
