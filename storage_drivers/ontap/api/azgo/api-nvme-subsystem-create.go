// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemCreateRequest is a structure to represent a nvme-subsystem-create Request ZAPI object
type NvmeSubsystemCreateRequest struct {
	XMLName          xml.Name             `xml:"nvme-subsystem-create"`
	CommentPtr       *string              `xml:"comment"`
	DeleteOnUnmapPtr *bool                `xml:"delete-on-unmap"`
	OstypePtr        *NvmeSubsystemOsType `xml:"ostype"`
	ReturnRecordPtr  *bool                `xml:"return-record"`
	SubsystemPtr     *string              `xml:"subsystem"`
}

// NvmeSubsystemCreateResponse is a structure to represent a nvme-subsystem-create Response ZAPI object
type NvmeSubsystemCreateResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeSubsystemCreateResponseResult `xml:"results"`
}

// NewNvmeSubsystemCreateResponse is a factory method for creating new instances of NvmeSubsystemCreateResponse objects
func NewNvmeSubsystemCreateResponse() *NvmeSubsystemCreateResponse {
	return &NvmeSubsystemCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemCreateResponseResult is a structure to represent a nvme-subsystem-create Response Result ZAPI object
type NvmeSubsystemCreateResponseResult struct {
	XMLName          xml.Name                                 `xml:"results"`
	ResultStatusAttr string                                   `xml:"status,attr"`
	ResultReasonAttr string                                   `xml:"reason,attr"`
	ResultErrnoAttr  string                                   `xml:"errno,attr"`
	ResultPtr        *NvmeSubsystemCreateResponseResultResult `xml:"result"`
}

// NewNvmeSubsystemCreateRequest is a factory method for creating new instances of NvmeSubsystemCreateRequest objects
func NewNvmeSubsystemCreateRequest() *NvmeSubsystemCreateRequest {
	return &NvmeSubsystemCreateRequest{}
}

// NewNvmeSubsystemCreateResponseResult is a factory method for creating new instances of NvmeSubsystemCreateResponseResult objects
func NewNvmeSubsystemCreateResponseResult() *NvmeSubsystemCreateResponseResult {
	return &NvmeSubsystemCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemCreateRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemCreateRequest", NewNvmeSubsystemCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemCreateResponse), err
}

// Comment is a 'getter' method
func (o *NvmeSubsystemCreateRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateRequest) SetComment(newValue string) *NvmeSubsystemCreateRequest {
	o.CommentPtr = &newValue
	return o
}

// DeleteOnUnmap is a 'getter' method
func (o *NvmeSubsystemCreateRequest) DeleteOnUnmap() bool {
	var r bool
	if o.DeleteOnUnmapPtr == nil {
		return r
	}
	r = *o.DeleteOnUnmapPtr
	return r
}

// SetDeleteOnUnmap is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateRequest) SetDeleteOnUnmap(newValue bool) *NvmeSubsystemCreateRequest {
	o.DeleteOnUnmapPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *NvmeSubsystemCreateRequest) Ostype() NvmeSubsystemOsType {
	var r NvmeSubsystemOsType
	if o.OstypePtr == nil {
		return r
	}
	r = *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateRequest) SetOstype(newValue NvmeSubsystemOsType) *NvmeSubsystemCreateRequest {
	o.OstypePtr = &newValue
	return o
}

// ReturnRecord is a 'getter' method
func (o *NvmeSubsystemCreateRequest) ReturnRecord() bool {
	var r bool
	if o.ReturnRecordPtr == nil {
		return r
	}
	r = *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateRequest) SetReturnRecord(newValue bool) *NvmeSubsystemCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemCreateRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateRequest) SetSubsystem(newValue string) *NvmeSubsystemCreateRequest {
	o.SubsystemPtr = &newValue
	return o
}

// NvmeSubsystemCreateResponseResultResult is a wrapper
type NvmeSubsystemCreateResponseResultResult struct {
	XMLName              xml.Name               `xml:"result"`
	NvmeSubsystemInfoPtr *NvmeSubsystemInfoType `xml:"nvme-subsystem-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemCreateResponseResultResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeSubsystemInfo is a 'getter' method
func (o *NvmeSubsystemCreateResponseResultResult) NvmeSubsystemInfo() NvmeSubsystemInfoType {
	var r NvmeSubsystemInfoType
	if o.NvmeSubsystemInfoPtr == nil {
		return r
	}
	r = *o.NvmeSubsystemInfoPtr
	return r
}

// SetNvmeSubsystemInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateResponseResultResult) SetNvmeSubsystemInfo(newValue NvmeSubsystemInfoType) *NvmeSubsystemCreateResponseResultResult {
	o.NvmeSubsystemInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *NvmeSubsystemCreateResponseResultResult) values() NvmeSubsystemInfoType {
	var r NvmeSubsystemInfoType
	if o.NvmeSubsystemInfoPtr == nil {
		return r
	}
	r = *o.NvmeSubsystemInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateResponseResultResult) setValues(newValue NvmeSubsystemInfoType) *NvmeSubsystemCreateResponseResultResult {
	o.NvmeSubsystemInfoPtr = &newValue
	return o
}

// Result is a 'getter' method
func (o *NvmeSubsystemCreateResponseResult) Result() NvmeSubsystemCreateResponseResultResult {
	var r NvmeSubsystemCreateResponseResultResult
	if o.ResultPtr == nil {
		return r
	}
	r = *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemCreateResponseResult) SetResult(newValue NvmeSubsystemCreateResponseResultResult) *NvmeSubsystemCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
