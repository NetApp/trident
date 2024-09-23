// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeModifyIterAsyncRequest is a structure to represent a volume-modify-iter-async Request ZAPI object
type VolumeModifyIterAsyncRequest struct {
	XMLName              xml.Name                                `xml:"volume-modify-iter-async"`
	AttributesPtr        *VolumeModifyIterAsyncRequestAttributes `xml:"attributes"`
	ContinueOnFailurePtr *bool                                   `xml:"continue-on-failure"`
	MaxFailureCountPtr   *int                                    `xml:"max-failure-count"`
	MaxRecordsPtr        *int                                    `xml:"max-records"`
	QueryPtr             *VolumeModifyIterAsyncRequestQuery      `xml:"query"`
	ReturnFailureListPtr *bool                                   `xml:"return-failure-list"`
	ReturnSuccessListPtr *bool                                   `xml:"return-success-list"`
	TagPtr               *string                                 `xml:"tag"`
}

// VolumeModifyIterAsyncResponse is a structure to represent a volume-modify-iter-async Response ZAPI object
type VolumeModifyIterAsyncResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          VolumeModifyIterAsyncResponseResult `xml:"results"`
}

// NewVolumeModifyIterAsyncResponse is a factory method for creating new instances of VolumeModifyIterAsyncResponse objects
func NewVolumeModifyIterAsyncResponse() *VolumeModifyIterAsyncResponse {
	return &VolumeModifyIterAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeModifyIterAsyncResponseResult is a structure to represent a volume-modify-iter-async Response Result ZAPI object
type VolumeModifyIterAsyncResponseResult struct {
	XMLName          xml.Name                                        `xml:"results"`
	ResultStatusAttr string                                          `xml:"status,attr"`
	ResultReasonAttr string                                          `xml:"reason,attr"`
	ResultErrnoAttr  string                                          `xml:"errno,attr"`
	FailureListPtr   *VolumeModifyIterAsyncResponseResultFailureList `xml:"failure-list"`
	NextTagPtr       *string                                         `xml:"next-tag"`
	NumFailedPtr     *int                                            `xml:"num-failed"`
	NumSucceededPtr  *int                                            `xml:"num-succeeded"`
	SuccessListPtr   *VolumeModifyIterAsyncResponseResultSuccessList `xml:"success-list"`
}

// NewVolumeModifyIterAsyncRequest is a factory method for creating new instances of VolumeModifyIterAsyncRequest objects
func NewVolumeModifyIterAsyncRequest() *VolumeModifyIterAsyncRequest {
	return &VolumeModifyIterAsyncRequest{}
}

// NewVolumeModifyIterAsyncResponseResult is a factory method for creating new instances of VolumeModifyIterAsyncResponseResult objects
func NewVolumeModifyIterAsyncResponseResult() *VolumeModifyIterAsyncResponseResult {
	return &VolumeModifyIterAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeModifyIterAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeModifyIterAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeModifyIterAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeModifyIterAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeModifyIterAsyncRequest", NewVolumeModifyIterAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeModifyIterAsyncResponse), err
}

// VolumeModifyIterAsyncRequestAttributes is a wrapper
type VolumeModifyIterAsyncRequestAttributes struct {
	XMLName             xml.Name              `xml:"attributes"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncRequestAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeModifyIterAsyncRequestAttributes) VolumeAttributes() VolumeAttributesType {
	var r VolumeAttributesType
	if o.VolumeAttributesPtr == nil {
		return r
	}
	r = *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequestAttributes) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeModifyIterAsyncRequestAttributes {
	o.VolumeAttributesPtr = &newValue
	return o
}

// Attributes is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) Attributes() VolumeModifyIterAsyncRequestAttributes {
	var r VolumeModifyIterAsyncRequestAttributes
	if o.AttributesPtr == nil {
		return r
	}
	r = *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetAttributes(newValue VolumeModifyIterAsyncRequestAttributes) *VolumeModifyIterAsyncRequest {
	o.AttributesPtr = &newValue
	return o
}

// ContinueOnFailure is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) ContinueOnFailure() bool {
	var r bool
	if o.ContinueOnFailurePtr == nil {
		return r
	}
	r = *o.ContinueOnFailurePtr
	return r
}

// SetContinueOnFailure is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetContinueOnFailure(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ContinueOnFailurePtr = &newValue
	return o
}

// MaxFailureCount is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) MaxFailureCount() int {
	var r int
	if o.MaxFailureCountPtr == nil {
		return r
	}
	r = *o.MaxFailureCountPtr
	return r
}

// SetMaxFailureCount is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetMaxFailureCount(newValue int) *VolumeModifyIterAsyncRequest {
	o.MaxFailureCountPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetMaxRecords(newValue int) *VolumeModifyIterAsyncRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VolumeModifyIterAsyncRequestQuery is a wrapper
type VolumeModifyIterAsyncRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeModifyIterAsyncRequestQuery) VolumeAttributes() VolumeAttributesType {
	var r VolumeAttributesType
	if o.VolumeAttributesPtr == nil {
		return r
	}
	r = *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequestQuery) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeModifyIterAsyncRequestQuery {
	o.VolumeAttributesPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) Query() VolumeModifyIterAsyncRequestQuery {
	var r VolumeModifyIterAsyncRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetQuery(newValue VolumeModifyIterAsyncRequestQuery) *VolumeModifyIterAsyncRequest {
	o.QueryPtr = &newValue
	return o
}

// ReturnFailureList is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) ReturnFailureList() bool {
	var r bool
	if o.ReturnFailureListPtr == nil {
		return r
	}
	r = *o.ReturnFailureListPtr
	return r
}

// SetReturnFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetReturnFailureList(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ReturnFailureListPtr = &newValue
	return o
}

// ReturnSuccessList is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) ReturnSuccessList() bool {
	var r bool
	if o.ReturnSuccessListPtr == nil {
		return r
	}
	r = *o.ReturnSuccessListPtr
	return r
}

// SetReturnSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetReturnSuccessList(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ReturnSuccessListPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VolumeModifyIterAsyncRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetTag(newValue string) *VolumeModifyIterAsyncRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeModifyIterAsyncResponseResultFailureList is a wrapper
type VolumeModifyIterAsyncResponseResultFailureList struct {
	XMLName                      xml.Name                        `xml:"failure-list"`
	VolumeModifyIterAsyncInfoPtr []VolumeModifyIterAsyncInfoType `xml:"volume-modify-iter-async-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponseResultFailureList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeModifyIterAsyncInfo is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResultFailureList) VolumeModifyIterAsyncInfo() []VolumeModifyIterAsyncInfoType {
	r := o.VolumeModifyIterAsyncInfoPtr
	return r
}

// SetVolumeModifyIterAsyncInfo is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResultFailureList) SetVolumeModifyIterAsyncInfo(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResultFailureList {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterAsyncInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResultFailureList) values() []VolumeModifyIterAsyncInfoType {
	r := o.VolumeModifyIterAsyncInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResultFailureList) setValues(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResultFailureList {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterAsyncInfoPtr = newSlice
	return o
}

// FailureList is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResult) FailureList() VolumeModifyIterAsyncResponseResultFailureList {
	var r VolumeModifyIterAsyncResponseResultFailureList
	if o.FailureListPtr == nil {
		return r
	}
	r = *o.FailureListPtr
	return r
}

// SetFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetFailureList(newValue VolumeModifyIterAsyncResponseResultFailureList) *VolumeModifyIterAsyncResponseResult {
	o.FailureListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNextTag(newValue string) *VolumeModifyIterAsyncResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumFailed is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResult) NumFailed() int {
	var r int
	if o.NumFailedPtr == nil {
		return r
	}
	r = *o.NumFailedPtr
	return r
}

// SetNumFailed is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNumFailed(newValue int) *VolumeModifyIterAsyncResponseResult {
	o.NumFailedPtr = &newValue
	return o
}

// NumSucceeded is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResult) NumSucceeded() int {
	var r int
	if o.NumSucceededPtr == nil {
		return r
	}
	r = *o.NumSucceededPtr
	return r
}

// SetNumSucceeded is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNumSucceeded(newValue int) *VolumeModifyIterAsyncResponseResult {
	o.NumSucceededPtr = &newValue
	return o
}

// VolumeModifyIterAsyncResponseResultSuccessList is a wrapper
type VolumeModifyIterAsyncResponseResultSuccessList struct {
	XMLName                      xml.Name                        `xml:"success-list"`
	VolumeModifyIterAsyncInfoPtr []VolumeModifyIterAsyncInfoType `xml:"volume-modify-iter-async-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponseResultSuccessList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeModifyIterAsyncInfo is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResultSuccessList) VolumeModifyIterAsyncInfo() []VolumeModifyIterAsyncInfoType {
	r := o.VolumeModifyIterAsyncInfoPtr
	return r
}

// SetVolumeModifyIterAsyncInfo is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResultSuccessList) SetVolumeModifyIterAsyncInfo(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResultSuccessList {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterAsyncInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResultSuccessList) values() []VolumeModifyIterAsyncInfoType {
	r := o.VolumeModifyIterAsyncInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResultSuccessList) setValues(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResultSuccessList {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterAsyncInfoPtr = newSlice
	return o
}

// SuccessList is a 'getter' method
func (o *VolumeModifyIterAsyncResponseResult) SuccessList() VolumeModifyIterAsyncResponseResultSuccessList {
	var r VolumeModifyIterAsyncResponseResultSuccessList
	if o.SuccessListPtr == nil {
		return r
	}
	r = *o.SuccessListPtr
	return r
}

// SetSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetSuccessList(newValue VolumeModifyIterAsyncResponseResultSuccessList) *VolumeModifyIterAsyncResponseResult {
	o.SuccessListPtr = &newValue
	return o
}
