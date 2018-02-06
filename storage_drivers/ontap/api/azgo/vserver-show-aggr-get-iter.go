// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VserverShowAggrGetIterRequest is a structure to represent a vserver-show-aggr-get-iter ZAPI request object
type VserverShowAggrGetIterRequest struct {
	XMLName xml.Name `xml:"vserver-show-aggr-get-iter"`

	DesiredAttributesPtr *ShowAggregatesType `xml:"desired-attributes>show-aggregates"`
	MaxRecordsPtr        *int                `xml:"max-records"`
	QueryPtr             *ShowAggregatesType `xml:"query>show-aggregates"`
	TagPtr               *string             `xml:"tag"`
	VserverPtr           *string             `xml:"vserver"`
}

// ToXML converts this object into an xml string representation
func (o *VserverShowAggrGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVserverShowAggrGetIterRequest is a factory method for creating new instances of VserverShowAggrGetIterRequest objects
func NewVserverShowAggrGetIterRequest() *VserverShowAggrGetIterRequest {
	return &VserverShowAggrGetIterRequest{}
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverShowAggrGetIterRequest) ExecuteUsing(zr *ZapiRunner) (VserverShowAggrGetIterResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VserverShowAggrGetIterRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	combined := NewVserverShowAggrGetIterResponse()
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

		var n VserverShowAggrGetIterResponse
		unmarshalErr := xml.Unmarshal(body, &n)
		if unmarshalErr != nil {
			log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
			//return *combined, unmarshalErr
		}
		if zr.DebugTraceFlags["api"] {
			log.Debugf("vserver-show-aggr-get-iter result:\n%s", n.Result)
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
func (o VserverShowAggrGetIterRequest) String() string {
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
	if o.VserverPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "vserver", *o.VserverPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("vserver: nil\n"))
	}
	return buffer.String()
}

// DesiredAttributes is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterRequest) DesiredAttributes() ShowAggregatesType {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetDesiredAttributes(newValue ShowAggregatesType) *VserverShowAggrGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetMaxRecords(newValue int) *VserverShowAggrGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// Query is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterRequest) Query() ShowAggregatesType {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetQuery(newValue ShowAggregatesType) *VserverShowAggrGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetTag(newValue string) *VserverShowAggrGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// Vserver is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterRequest) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetVserver(newValue string) *VserverShowAggrGetIterRequest {
	o.VserverPtr = &newValue
	return o
}

// VserverShowAggrGetIterResponse is a structure to represent a vserver-show-aggr-get-iter ZAPI response object
type VserverShowAggrGetIterResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VserverShowAggrGetIterResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VserverShowAggrGetIterResponseResult is a structure to represent a vserver-show-aggr-get-iter ZAPI object's result
type VserverShowAggrGetIterResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr  string               `xml:"status,attr"`
	ResultReasonAttr  string               `xml:"reason,attr"`
	ResultErrnoAttr   string               `xml:"errno,attr"`
	AttributesListPtr []ShowAggregatesType `xml:"attributes-list>show-aggregates"`
	NextTagPtr        *string              `xml:"next-tag"`
	NumRecordsPtr     *int                 `xml:"num-records"`
}

// ToXML converts this object into an xml string representation
func (o *VserverShowAggrGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVserverShowAggrGetIterResponse is a factory method for creating new instances of VserverShowAggrGetIterResponse objects
func NewVserverShowAggrGetIterResponse() *VserverShowAggrGetIterResponse {
	return &VserverShowAggrGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterResponseResult) String() string {
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
func (o *VserverShowAggrGetIterResponseResult) AttributesList() []ShowAggregatesType {
	r := o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetAttributesList(newValue []ShowAggregatesType) *VserverShowAggrGetIterResponseResult {
	newSlice := make([]ShowAggregatesType, len(newValue))
	copy(newSlice, newValue)
	o.AttributesListPtr = newSlice
	return o
}

// NextTag is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetNextTag(newValue string) *VserverShowAggrGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a fluent style 'getter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetNumRecords(newValue int) *VserverShowAggrGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
