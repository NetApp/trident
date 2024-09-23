// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeNamespaceCreateRequest is a structure to represent a nvme-namespace-create Request ZAPI object
type NvmeNamespaceCreateRequest struct {
	XMLName         xml.Name             `xml:"nvme-namespace-create"`
	ApplicationPtr  *string              `xml:"application"`
	BlockSizePtr    *VdiskBlockSizeType  `xml:"block-size"`
	CommentPtr      *string              `xml:"comment"`
	OstypePtr       *NvmeNamespaceOsType `xml:"ostype"`
	PathPtr         *LunPathType         `xml:"path"`
	ReturnRecordPtr *bool                `xml:"return-record"`
	SizePtr         *SizeType            `xml:"size"`
	UuidPtr         *UuidType            `xml:"uuid"`
}

// NvmeNamespaceCreateResponse is a structure to represent a nvme-namespace-create Response ZAPI object
type NvmeNamespaceCreateResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeNamespaceCreateResponseResult `xml:"results"`
}

// NewNvmeNamespaceCreateResponse is a factory method for creating new instances of NvmeNamespaceCreateResponse objects
func NewNvmeNamespaceCreateResponse() *NvmeNamespaceCreateResponse {
	return &NvmeNamespaceCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeNamespaceCreateResponseResult is a structure to represent a nvme-namespace-create Response Result ZAPI object
type NvmeNamespaceCreateResponseResult struct {
	XMLName          xml.Name                                 `xml:"results"`
	ResultStatusAttr string                                   `xml:"status,attr"`
	ResultReasonAttr string                                   `xml:"reason,attr"`
	ResultErrnoAttr  string                                   `xml:"errno,attr"`
	ResultPtr        *NvmeNamespaceCreateResponseResultResult `xml:"result"`
}

// NewNvmeNamespaceCreateRequest is a factory method for creating new instances of NvmeNamespaceCreateRequest objects
func NewNvmeNamespaceCreateRequest() *NvmeNamespaceCreateRequest {
	return &NvmeNamespaceCreateRequest{}
}

// NewNvmeNamespaceCreateResponseResult is a factory method for creating new instances of NvmeNamespaceCreateResponseResult objects
func NewNvmeNamespaceCreateResponseResult() *NvmeNamespaceCreateResponseResult {
	return &NvmeNamespaceCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceCreateRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeNamespaceCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeNamespaceCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeNamespaceCreateRequest", NewNvmeNamespaceCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeNamespaceCreateResponse), err
}

// Application is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Application() string {
	var r string
	if o.ApplicationPtr == nil {
		return r
	}
	r = *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetApplication(newValue string) *NvmeNamespaceCreateRequest {
	o.ApplicationPtr = &newValue
	return o
}

// BlockSize is a 'getter' method
func (o *NvmeNamespaceCreateRequest) BlockSize() VdiskBlockSizeType {
	var r VdiskBlockSizeType
	if o.BlockSizePtr == nil {
		return r
	}
	r = *o.BlockSizePtr
	return r
}

// SetBlockSize is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetBlockSize(newValue VdiskBlockSizeType) *NvmeNamespaceCreateRequest {
	o.BlockSizePtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetComment(newValue string) *NvmeNamespaceCreateRequest {
	o.CommentPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Ostype() NvmeNamespaceOsType {
	var r NvmeNamespaceOsType
	if o.OstypePtr == nil {
		return r
	}
	r = *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetOstype(newValue NvmeNamespaceOsType) *NvmeNamespaceCreateRequest {
	o.OstypePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetPath(newValue LunPathType) *NvmeNamespaceCreateRequest {
	o.PathPtr = &newValue
	return o
}

// ReturnRecord is a 'getter' method
func (o *NvmeNamespaceCreateRequest) ReturnRecord() bool {
	var r bool
	if o.ReturnRecordPtr == nil {
		return r
	}
	r = *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetReturnRecord(newValue bool) *NvmeNamespaceCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Size() SizeType {
	var r SizeType
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetSize(newValue SizeType) *NvmeNamespaceCreateRequest {
	o.SizePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *NvmeNamespaceCreateRequest) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateRequest) SetUuid(newValue UuidType) *NvmeNamespaceCreateRequest {
	o.UuidPtr = &newValue
	return o
}

// NvmeNamespaceCreateResponseResultResult is a wrapper
type NvmeNamespaceCreateResponseResultResult struct {
	XMLName              xml.Name               `xml:"result"`
	NvmeNamespaceInfoPtr *NvmeNamespaceInfoType `xml:"nvme-namespace-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceCreateResponseResultResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeNamespaceInfo is a 'getter' method
func (o *NvmeNamespaceCreateResponseResultResult) NvmeNamespaceInfo() NvmeNamespaceInfoType {
	var r NvmeNamespaceInfoType
	if o.NvmeNamespaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeNamespaceInfoPtr
	return r
}

// SetNvmeNamespaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateResponseResultResult) SetNvmeNamespaceInfo(newValue NvmeNamespaceInfoType) *NvmeNamespaceCreateResponseResultResult {
	o.NvmeNamespaceInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *NvmeNamespaceCreateResponseResultResult) values() NvmeNamespaceInfoType {
	var r NvmeNamespaceInfoType
	if o.NvmeNamespaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeNamespaceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateResponseResultResult) setValues(newValue NvmeNamespaceInfoType) *NvmeNamespaceCreateResponseResultResult {
	o.NvmeNamespaceInfoPtr = &newValue
	return o
}

// Result is a 'getter' method
func (o *NvmeNamespaceCreateResponseResult) Result() NvmeNamespaceCreateResponseResultResult {
	var r NvmeNamespaceCreateResponseResultResult
	if o.ResultPtr == nil {
		return r
	}
	r = *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceCreateResponseResult) SetResult(newValue NvmeNamespaceCreateResponseResultResult) *NvmeNamespaceCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
