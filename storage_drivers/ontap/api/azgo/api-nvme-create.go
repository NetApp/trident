// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeCreateRequest is a structure to represent a nvme-create Request ZAPI object
type NvmeCreateRequest struct {
	XMLName         xml.Name `xml:"nvme-create"`
	IsAvailablePtr  *bool    `xml:"is-available"`
	ReturnRecordPtr *bool    `xml:"return-record"`
}

// NvmeCreateResponse is a structure to represent a nvme-create Response ZAPI object
type NvmeCreateResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          NvmeCreateResponseResult `xml:"results"`
}

// NewNvmeCreateResponse is a factory method for creating new instances of NvmeCreateResponse objects
func NewNvmeCreateResponse() *NvmeCreateResponse {
	return &NvmeCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeCreateResponseResult is a structure to represent a nvme-create Response Result ZAPI object
type NvmeCreateResponseResult struct {
	XMLName          xml.Name                        `xml:"results"`
	ResultStatusAttr string                          `xml:"status,attr"`
	ResultReasonAttr string                          `xml:"reason,attr"`
	ResultErrnoAttr  string                          `xml:"errno,attr"`
	ResultPtr        *NvmeCreateResponseResultResult `xml:"result"`
}

// NewNvmeCreateRequest is a factory method for creating new instances of NvmeCreateRequest objects
func NewNvmeCreateRequest() *NvmeCreateRequest {
	return &NvmeCreateRequest{}
}

// NewNvmeCreateResponseResult is a factory method for creating new instances of NvmeCreateResponseResult objects
func NewNvmeCreateResponseResult() *NvmeCreateResponseResult {
	return &NvmeCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeCreateRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeCreateRequest", NewNvmeCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeCreateResponse), err
}

// IsAvailable is a 'getter' method
func (o *NvmeCreateRequest) IsAvailable() bool {
	var r bool
	if o.IsAvailablePtr == nil {
		return r
	}
	r = *o.IsAvailablePtr
	return r
}

// SetIsAvailable is a fluent style 'setter' method that can be chained
func (o *NvmeCreateRequest) SetIsAvailable(newValue bool) *NvmeCreateRequest {
	o.IsAvailablePtr = &newValue
	return o
}

// ReturnRecord is a 'getter' method
func (o *NvmeCreateRequest) ReturnRecord() bool {
	var r bool
	if o.ReturnRecordPtr == nil {
		return r
	}
	r = *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *NvmeCreateRequest) SetReturnRecord(newValue bool) *NvmeCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// NvmeCreateResponseResultResult is a wrapper
type NvmeCreateResponseResultResult struct {
	XMLName                  xml.Name                   `xml:"result"`
	NvmeTargetServiceInfoPtr *NvmeTargetServiceInfoType `xml:"nvme-target-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeCreateResponseResultResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetServiceInfo is a 'getter' method
func (o *NvmeCreateResponseResultResult) NvmeTargetServiceInfo() NvmeTargetServiceInfoType {
	var r NvmeTargetServiceInfoType
	if o.NvmeTargetServiceInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetServiceInfoPtr
	return r
}

// SetNvmeTargetServiceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeCreateResponseResultResult) SetNvmeTargetServiceInfo(newValue NvmeTargetServiceInfoType) *NvmeCreateResponseResultResult {
	o.NvmeTargetServiceInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *NvmeCreateResponseResultResult) values() NvmeTargetServiceInfoType {
	var r NvmeTargetServiceInfoType
	if o.NvmeTargetServiceInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetServiceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeCreateResponseResultResult) setValues(newValue NvmeTargetServiceInfoType) *NvmeCreateResponseResultResult {
	o.NvmeTargetServiceInfoPtr = &newValue
	return o
}

// Result is a 'getter' method
func (o *NvmeCreateResponseResult) Result() NvmeCreateResponseResultResult {
	var r NvmeCreateResponseResultResult
	if o.ResultPtr == nil {
		return r
	}
	r = *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *NvmeCreateResponseResult) SetResult(newValue NvmeCreateResponseResultResult) *NvmeCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
