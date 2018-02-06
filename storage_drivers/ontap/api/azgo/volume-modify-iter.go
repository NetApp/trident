// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeModifyIterRequest is a structure to represent a volume-modify-iter ZAPI request object
type VolumeModifyIterRequest struct {
	XMLName xml.Name `xml:"volume-modify-iter"`

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
func (o *VolumeModifyIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeModifyIterRequest is a factory method for creating new instances of VolumeModifyIterRequest objects
func NewVolumeModifyIterRequest() *VolumeModifyIterRequest { return &VolumeModifyIterRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeModifyIterRequest) ExecuteUsing(zr *ZapiRunner) (VolumeModifyIterResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeModifyIterRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeModifyIterResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeModifyIterResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeModifyIterResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeModifyIterResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-modify-iter result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterRequest) String() string {
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
func (o *VolumeModifyIterRequest) Attributes() VolumeAttributesType {
	r := *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetAttributes(newValue VolumeAttributesType) *VolumeModifyIterRequest {
	o.AttributesPtr = &newValue
	return o
}

// ContinueOnFailure is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) ContinueOnFailure() bool {
	r := *o.ContinueOnFailurePtr
	return r
}

// SetContinueOnFailure is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetContinueOnFailure(newValue bool) *VolumeModifyIterRequest {
	o.ContinueOnFailurePtr = &newValue
	return o
}

// MaxFailureCount is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) MaxFailureCount() int {
	r := *o.MaxFailureCountPtr
	return r
}

// SetMaxFailureCount is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetMaxFailureCount(newValue int) *VolumeModifyIterRequest {
	o.MaxFailureCountPtr = &newValue
	return o
}

// MaxRecords is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetMaxRecords(newValue int) *VolumeModifyIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// Query is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) Query() VolumeAttributesType {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetQuery(newValue VolumeAttributesType) *VolumeModifyIterRequest {
	o.QueryPtr = &newValue
	return o
}

// ReturnFailureList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) ReturnFailureList() bool {
	r := *o.ReturnFailureListPtr
	return r
}

// SetReturnFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetReturnFailureList(newValue bool) *VolumeModifyIterRequest {
	o.ReturnFailureListPtr = &newValue
	return o
}

// ReturnSuccessList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) ReturnSuccessList() bool {
	r := *o.ReturnSuccessListPtr
	return r
}

// SetReturnSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetReturnSuccessList(newValue bool) *VolumeModifyIterRequest {
	o.ReturnSuccessListPtr = &newValue
	return o
}

// Tag is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetTag(newValue string) *VolumeModifyIterRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeModifyIterResponse is a structure to represent a volume-modify-iter ZAPI response object
type VolumeModifyIterResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeModifyIterResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeModifyIterResponseResult is a structure to represent a volume-modify-iter ZAPI object's result
type VolumeModifyIterResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string                     `xml:"status,attr"`
	ResultReasonAttr string                     `xml:"reason,attr"`
	ResultErrnoAttr  string                     `xml:"errno,attr"`
	FailureListPtr   []VolumeModifyIterInfoType `xml:"failure-list>volume-modify-iter-info"`
	NextTagPtr       *string                    `xml:"next-tag"`
	NumFailedPtr     *int                       `xml:"num-failed"`
	NumSucceededPtr  *int                       `xml:"num-succeeded"`
	SuccessListPtr   []VolumeModifyIterInfoType `xml:"success-list>volume-modify-iter-info"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeModifyIterResponse is a factory method for creating new instances of VolumeModifyIterResponse objects
func NewVolumeModifyIterResponse() *VolumeModifyIterResponse { return &VolumeModifyIterResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponseResult) String() string {
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
func (o *VolumeModifyIterResponseResult) FailureList() []VolumeModifyIterInfoType {
	r := o.FailureListPtr
	return r
}

// SetFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetFailureList(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResult {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FailureListPtr = newSlice
	return o
}

// NextTag is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNextTag(newValue string) *VolumeModifyIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumFailed is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterResponseResult) NumFailed() int {
	r := *o.NumFailedPtr
	return r
}

// SetNumFailed is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNumFailed(newValue int) *VolumeModifyIterResponseResult {
	o.NumFailedPtr = &newValue
	return o
}

// NumSucceeded is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterResponseResult) NumSucceeded() int {
	r := *o.NumSucceededPtr
	return r
}

// SetNumSucceeded is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNumSucceeded(newValue int) *VolumeModifyIterResponseResult {
	o.NumSucceededPtr = &newValue
	return o
}

// SuccessList is a fluent style 'getter' method that can be chained
func (o *VolumeModifyIterResponseResult) SuccessList() []VolumeModifyIterInfoType {
	r := o.SuccessListPtr
	return r
}

// SetSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetSuccessList(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResult {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SuccessListPtr = newSlice
	return o
}
