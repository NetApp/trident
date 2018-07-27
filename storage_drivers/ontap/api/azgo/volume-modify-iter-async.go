// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeModifyIterAsyncRequest is a structure to represent a volume-modify-iter-async ZAPI request object
type VolumeModifyIterAsyncRequest struct {
	XMLName xml.Name `xml:"volume-modify-iter-async"`

	AttributesPtr        *VolumeAttributesType `xml:"attributes>volume-attributes"`
	ContinueOnFailurePtr *bool                 `xml:"continue-on-failure"`
	MaxFailureCountPtr   *int                  `xml:"max-failure-count"`
	MaxRecordsPtr        *int                  `xml:"max-records"`
	QueryPtr             *VolumeAttributesType `xml:"query>volume-attributes"`
	ReturnFailureListPtr *bool                 `xml:"return-failure-list"`
	ReturnSuccessListPtr *bool                 `xml:"return-success-list"`
	TagPtr               *string               `xml:"tag"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeModifyIterAsyncRequest is a factory method for creating new instances of VolumeModifyIterAsyncRequest objects
func NewVolumeModifyIterAsyncRequest() *VolumeModifyIterAsyncRequest {
	return &VolumeModifyIterAsyncRequest{}
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeModifyIterAsyncRequest) ExecuteUsing(zr *ZapiRunner) (VolumeModifyIterAsyncResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeModifyIterAsyncRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeModifyIterAsyncResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeModifyIterAsyncResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeModifyIterAsyncResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return VolumeModifyIterAsyncResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-modify-iter-async result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncRequest) String() string {
	var buffer bytes.Buffer
	if o.AttributesPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "attributes", *o.AttributesPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("attributes: nil\n"))
	}
	if o.ContinueOnFailurePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "continue-on-failure", *o.ContinueOnFailurePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("continue-on-failure: nil\n"))
	}
	if o.MaxFailureCountPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-failure-count", *o.MaxFailureCountPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-failure-count: nil\n"))
	}
	if o.MaxRecordsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-records", *o.MaxRecordsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-records: nil\n"))
	}
	if o.QueryPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "query", *o.QueryPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("query: nil\n"))
	}
	if o.ReturnFailureListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "return-failure-list", *o.ReturnFailureListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("return-failure-list: nil\n"))
	}
	if o.ReturnSuccessListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "return-success-list", *o.ReturnSuccessListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("return-success-list: nil\n"))
	}
	if o.TagPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "tag", *o.TagPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("tag: nil\n"))
	}
	return buffer.String()
}

// Attributes is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) Attributes() VolumeAttributesType {
	r := *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetAttributes(newValue VolumeAttributesType) *VolumeModifyIterAsyncRequest {
	o.AttributesPtr = &newValue
	return o
}

// ContinueOnFailure is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) ContinueOnFailure() bool {
	r := *o.ContinueOnFailurePtr
	return r
}

// SetContinueOnFailure is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetContinueOnFailure(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ContinueOnFailurePtr = &newValue
	return o
}

// MaxFailureCount is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) MaxFailureCount() int {
	r := *o.MaxFailureCountPtr
	return r
}

// SetMaxFailureCount is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetMaxFailureCount(newValue int) *VolumeModifyIterAsyncRequest {
	o.MaxFailureCountPtr = &newValue
	return o
}

// MaxRecords is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetMaxRecords(newValue int) *VolumeModifyIterAsyncRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// Query is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) Query() VolumeAttributesType {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetQuery(newValue VolumeAttributesType) *VolumeModifyIterAsyncRequest {
	o.QueryPtr = &newValue
	return o
}

// ReturnFailureList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) ReturnFailureList() bool {
	r := *o.ReturnFailureListPtr
	return r
}

// SetReturnFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetReturnFailureList(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ReturnFailureListPtr = &newValue
	return o
}

// ReturnSuccessList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) ReturnSuccessList() bool {
	r := *o.ReturnSuccessListPtr
	return r
}

// SetReturnSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetReturnSuccessList(newValue bool) *VolumeModifyIterAsyncRequest {
	o.ReturnSuccessListPtr = &newValue
	return o
}

// Tag is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncRequest) SetTag(newValue string) *VolumeModifyIterAsyncRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeModifyIterAsyncResponse is a structure to represent a volume-modify-iter-async ZAPI response object
type VolumeModifyIterAsyncResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeModifyIterAsyncResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeModifyIterAsyncResponseResult is a structure to represent a volume-modify-iter-async ZAPI object's result
type VolumeModifyIterAsyncResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string                          `xml:"status,attr"`
	ResultReasonAttr string                          `xml:"reason,attr"`
	ResultErrnoAttr  string                          `xml:"errno,attr"`
	FailureListPtr   []VolumeModifyIterAsyncInfoType `xml:"failure-list>volume-modify-iter-async-info"`
	NextTagPtr       *string                         `xml:"next-tag"`
	NumFailedPtr     *int                            `xml:"num-failed"`
	NumSucceededPtr  *int                            `xml:"num-succeeded"`
	SuccessListPtr   []VolumeModifyIterAsyncInfoType `xml:"success-list>volume-modify-iter-async-info"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeModifyIterAsyncResponse is a factory method for creating new instances of VolumeModifyIterAsyncResponse objects
func NewVolumeModifyIterAsyncResponse() *VolumeModifyIterAsyncResponse {
	return &VolumeModifyIterAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterAsyncResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.FailureListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "failure-list", o.FailureListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("failure-list: nil\n"))
	}
	if o.NextTagPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "next-tag", *o.NextTagPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("next-tag: nil\n"))
	}
	if o.NumFailedPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "num-failed", *o.NumFailedPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("num-failed: nil\n"))
	}
	if o.NumSucceededPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "num-succeeded", *o.NumSucceededPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("num-succeeded: nil\n"))
	}
	if o.SuccessListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "success-list", o.SuccessListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("success-list: nil\n"))
	}
	return buffer.String()
}

// FailureList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) FailureList() []VolumeModifyIterAsyncInfoType {
	r := o.FailureListPtr
	return r
}

// SetFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetFailureList(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResult {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FailureListPtr = newSlice
	return o
}

// NextTag is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNextTag(newValue string) *VolumeModifyIterAsyncResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumFailed is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) NumFailed() int {
	r := *o.NumFailedPtr
	return r
}

// SetNumFailed is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNumFailed(newValue int) *VolumeModifyIterAsyncResponseResult {
	o.NumFailedPtr = &newValue
	return o
}

// NumSucceeded is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) NumSucceeded() int {
	r := *o.NumSucceededPtr
	return r
}

// SetNumSucceeded is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetNumSucceeded(newValue int) *VolumeModifyIterAsyncResponseResult {
	o.NumSucceededPtr = &newValue
	return o
}

// SuccessList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SuccessList() []VolumeModifyIterAsyncInfoType {
	r := o.SuccessListPtr
	return r
}

// SetSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterAsyncResponseResult) SetSuccessList(newValue []VolumeModifyIterAsyncInfoType) *VolumeModifyIterAsyncResponseResult {
	newSlice := make([]VolumeModifyIterAsyncInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SuccessListPtr = newSlice
	return o
}
