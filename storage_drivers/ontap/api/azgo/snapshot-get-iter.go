// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// SnapshotGetIterRequest is a structure to represent a snapshot-get-iter ZAPI request object
type SnapshotGetIterRequest struct {
	XMLName xml.Name `xml:"snapshot-get-iter"`

	DesiredAttributesPtr *SnapshotInfoType `xml:"desired-attributes>snapshot-info"`
	MaxRecordsPtr        *int              `xml:"max-records"`
	QueryPtr             *SnapshotInfoType `xml:"query>snapshot-info"`
	TagPtr               *string           `xml:"tag"`
}

// ToXML converts this object into an xml string representation
func (o *SnapshotGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSnapshotGetIterRequest is a factory method for creating new instances of SnapshotGetIterRequest objects
func NewSnapshotGetIterRequest() *SnapshotGetIterRequest { return &SnapshotGetIterRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapshotGetIterRequest) ExecuteUsing(zr *ZapiRunner) (SnapshotGetIterResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SnapshotGetIterRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	combined := NewSnapshotGetIterResponse()
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

		var n SnapshotGetIterResponse
		unmarshalErr := xml.Unmarshal(body, &n)
		if unmarshalErr != nil {
			log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
			//return *combined, unmarshalErr
		}
		if zr.DebugTraceFlags["api"] {
			log.Debugf("snapshot-get-iter result:\n%s", n.Result)
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
func (o SnapshotGetIterRequest) String() string {
	var buffer bytes.Buffer
	if o.DesiredAttributesPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "desired-attributes", *o.DesiredAttributesPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("desired-attributes: nil\n"))
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
func (o *SnapshotGetIterRequest) DesiredAttributes() SnapshotInfoType {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetDesiredAttributes(newValue SnapshotInfoType) *SnapshotGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetMaxRecords(newValue int) *SnapshotGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// Query is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterRequest) Query() SnapshotInfoType {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetQuery(newValue SnapshotInfoType) *SnapshotGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetTag(newValue string) *SnapshotGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapshotGetIterResponse is a structure to represent a snapshot-get-iter ZAPI response object
type SnapshotGetIterResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SnapshotGetIterResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SnapshotGetIterResponseResult is a structure to represent a snapshot-get-iter ZAPI object's result
type SnapshotGetIterResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr  string             `xml:"status,attr"`
	ResultReasonAttr  string             `xml:"reason,attr"`
	ResultErrnoAttr   string             `xml:"errno,attr"`
	AttributesListPtr []SnapshotInfoType `xml:"attributes-list>snapshot-info"`
	NextTagPtr        *string            `xml:"next-tag"`
	NumRecordsPtr     *int               `xml:"num-records"`
	VolumeErrorsPtr   []VolumeErrorType  `xml:"volume-errors>volume-error"`
}

// ToXML converts this object into an xml string representation
func (o *SnapshotGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSnapshotGetIterResponse is a factory method for creating new instances of SnapshotGetIterResponse objects
func NewSnapshotGetIterResponse() *SnapshotGetIterResponse { return &SnapshotGetIterResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponseResult) String() string {
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
	if o.VolumeErrorsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-errors", o.VolumeErrorsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-errors: nil\n"))
	}
	return buffer.String()
}

// AttributesList is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterResponseResult) AttributesList() []SnapshotInfoType {
	r := o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetAttributesList(newValue []SnapshotInfoType) *SnapshotGetIterResponseResult {
	newSlice := make([]SnapshotInfoType, len(newValue))
	copy(newSlice, newValue)
	o.AttributesListPtr = newSlice
	return o
}

// NextTag is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetNextTag(newValue string) *SnapshotGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetNumRecords(newValue int) *SnapshotGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}

// VolumeErrors is a fluent style 'getter' method that can be chained
func (o *SnapshotGetIterResponseResult) VolumeErrors() []VolumeErrorType {
	r := o.VolumeErrorsPtr
	return r
}

// SetVolumeErrors is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetVolumeErrors(newValue []VolumeErrorType) *SnapshotGetIterResponseResult {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorsPtr = newSlice
	return o
}
