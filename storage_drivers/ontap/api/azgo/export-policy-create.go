// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// ExportPolicyCreateRequest is a structure to represent a export-policy-create ZAPI request object
type ExportPolicyCreateRequest struct {
	XMLName xml.Name `xml:"export-policy-create"`

	PolicyNamePtr   *ExportPolicyNameType `xml:"policy-name"`
	ReturnRecordPtr *bool                 `xml:"return-record"`
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewExportPolicyCreateRequest is a factory method for creating new instances of ExportPolicyCreateRequest objects
func NewExportPolicyCreateRequest() *ExportPolicyCreateRequest { return &ExportPolicyCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ExportPolicyCreateRequest) ExecuteUsing(zr *ZapiRunner) (ExportPolicyCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "ExportPolicyCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return ExportPolicyCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return ExportPolicyCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n ExportPolicyCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return ExportPolicyCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("export-policy-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.PolicyNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "policy-name", *o.PolicyNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("policy-name: nil\n"))
	}
	if o.ReturnRecordPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "return-record", *o.ReturnRecordPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("return-record: nil\n"))
	}
	return buffer.String()
}

// PolicyName is a fluent style 'getter' method that can be chained
func (o *ExportPolicyCreateRequest) PolicyName() ExportPolicyNameType {
	r := *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportPolicyCreateRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// ReturnRecord is a fluent style 'getter' method that can be chained
func (o *ExportPolicyCreateRequest) ReturnRecord() bool {
	r := *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateRequest) SetReturnRecord(newValue bool) *ExportPolicyCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// ExportPolicyCreateResponse is a structure to represent a export-policy-create ZAPI response object
type ExportPolicyCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result ExportPolicyCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// ExportPolicyCreateResponseResult is a structure to represent a export-policy-create ZAPI object's result
type ExportPolicyCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string                `xml:"status,attr"`
	ResultReasonAttr string                `xml:"reason,attr"`
	ResultErrnoAttr  string                `xml:"errno,attr"`
	ResultPtr        *ExportPolicyInfoType `xml:"result>export-policy-info"`
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewExportPolicyCreateResponse is a factory method for creating new instances of ExportPolicyCreateResponse objects
func NewExportPolicyCreateResponse() *ExportPolicyCreateResponse { return &ExportPolicyCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ResultPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result", *o.ResultPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result: nil\n"))
	}
	return buffer.String()
}

// Result is a fluent style 'getter' method that can be chained
func (o *ExportPolicyCreateResponseResult) Result() ExportPolicyInfoType {
	r := *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateResponseResult) SetResult(newValue ExportPolicyInfoType) *ExportPolicyCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
