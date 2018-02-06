// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorGetIterRequest is a structure to represent a snapmirror-get-iter ZAPI request object
type SnapmirrorGetIterRequest struct {
	XMLName xml.Name `xml:"snapmirror-get-iter"`

	DesiredAttributesPtr *SnapmirrorInfoType `xml:"desired-attributes>snapmirror-info"`
	ExpandPtr            *bool               `xml:"expand"`
	MaxRecordsPtr        *int                `xml:"max-records"`
	QueryPtr             *SnapmirrorInfoType `xml:"query>snapmirror-info"`
	TagPtr               *string             `xml:"tag"`
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSnapmirrorGetIterRequest is a factory method for creating new instances of SnapmirrorGetIterRequest objects
func NewSnapmirrorGetIterRequest() *SnapmirrorGetIterRequest { return &SnapmirrorGetIterRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorGetIterRequest) ExecuteUsing(zr *ZapiRunner) (SnapmirrorGetIterResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SnapmirrorGetIterRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	combined := NewSnapmirrorGetIterResponse()
	var nextTagPtr *string
	done := false
	for done != true {

		resp, err := zr.SendZapi(o)
		if err != nil {
			log.Errorf("API invocation failed. %v", err.Error())
			return *combined, err
		}
		defer resp.Body.Close()
		body, readErr := ioutil.ReadAll(resp.Body)
		if readErr != nil {
			log.Errorf("Error reading response body. %v", readErr.Error())
			return *combined, readErr
		}
		if zr.DebugTraceFlags["api"] {
			log.Debugf("response Body:\n%s", string(body))
		}

		var n SnapmirrorGetIterResponse
		unmarshalErr := xml.Unmarshal(body, &n)
		if unmarshalErr != nil {
			log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
			//return *combined, unmarshalErr
		}
		if zr.DebugTraceFlags["api"] {
			log.Debugf("snapmirror-get-iter result:\n%s", n.Result)
		}

		if err == nil {
			nextTagPtr = n.Result.NextTagPtr
			if nextTagPtr == nil {
				done = true
			} else {
				o.SetTag(*nextTagPtr)
			}

			if n.Result.NumRecordsPtr == nil {
				done = true
			} else {
				recordsRead := n.Result.NumRecords()
				if recordsRead == 0 {
					done = true
				}
			}

			if n.Result.AttributesListPtr != nil {
				combined.Result.SetAttributesList(append(combined.Result.AttributesList(), n.Result.AttributesList()...))
			}

			if done == true {
				combined.Result.ResultErrnoAttr = n.Result.ResultErrnoAttr
				combined.Result.ResultReasonAttr = n.Result.ResultReasonAttr
				combined.Result.ResultStatusAttr = n.Result.ResultStatusAttr
				combined.Result.SetNumRecords(len(combined.Result.AttributesList()))
			}
		}
	}

	return *combined, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterRequest) String() string {
	var buffer bytes.Buffer
	if o.DesiredAttributesPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "desired-attributes", *o.DesiredAttributesPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("desired-attributes: nil\n"))
	}
	if o.ExpandPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "expand", *o.ExpandPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("expand: nil\n"))
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
	if o.TagPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "tag", *o.TagPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("tag: nil\n"))
	}
	return buffer.String()
}

// DesiredAttributes is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterRequest) DesiredAttributes() SnapmirrorInfoType {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetDesiredAttributes(newValue SnapmirrorInfoType) *SnapmirrorGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// Expand is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterRequest) Expand() bool {
	r := *o.ExpandPtr
	return r
}

// SetExpand is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetExpand(newValue bool) *SnapmirrorGetIterRequest {
	o.ExpandPtr = &newValue
	return o
}

// MaxRecords is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetMaxRecords(newValue int) *SnapmirrorGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// Query is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterRequest) Query() SnapmirrorInfoType {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetQuery(newValue SnapmirrorInfoType) *SnapmirrorGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetTag(newValue string) *SnapmirrorGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapmirrorGetIterResponse is a structure to represent a snapmirror-get-iter ZAPI response object
type SnapmirrorGetIterResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SnapmirrorGetIterResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SnapmirrorGetIterResponseResult is a structure to represent a snapmirror-get-iter ZAPI object's result
type SnapmirrorGetIterResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr  string               `xml:"status,attr"`
	ResultReasonAttr  string               `xml:"reason,attr"`
	ResultErrnoAttr   string               `xml:"errno,attr"`
	AttributesListPtr []SnapmirrorInfoType `xml:"attributes-list>snapmirror-info"`
	NextTagPtr        *string              `xml:"next-tag"`
	NumRecordsPtr     *int                 `xml:"num-records"`
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSnapmirrorGetIterResponse is a factory method for creating new instances of SnapmirrorGetIterResponse objects
func NewSnapmirrorGetIterResponse() *SnapmirrorGetIterResponse { return &SnapmirrorGetIterResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.AttributesListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "attributes-list", o.AttributesListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("attributes-list: nil\n"))
	}
	if o.NextTagPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "next-tag", *o.NextTagPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("next-tag: nil\n"))
	}
	if o.NumRecordsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "num-records", *o.NumRecordsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("num-records: nil\n"))
	}
	return buffer.String()
}

// AttributesList is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) AttributesList() []SnapmirrorInfoType {
	r := o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetAttributesList(newValue []SnapmirrorInfoType) *SnapmirrorGetIterResponseResult {
	newSlice := make([]SnapmirrorInfoType, len(newValue))
	copy(newSlice, newValue)
	o.AttributesListPtr = newSlice
	return o
}

// NextTag is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetNextTag(newValue string) *SnapmirrorGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a fluent style 'getter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetNumRecords(newValue int) *SnapmirrorGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
