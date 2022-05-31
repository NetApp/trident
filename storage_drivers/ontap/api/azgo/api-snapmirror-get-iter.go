// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorGetIterRequest is a structure to represent a snapmirror-get-iter Request ZAPI object
type SnapmirrorGetIterRequest struct {
	XMLName              xml.Name                                   `xml:"snapmirror-get-iter"`
	DesiredAttributesPtr *SnapmirrorGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	ExpandPtr            *bool                                      `xml:"expand"`
	MaxRecordsPtr        *int                                       `xml:"max-records"`
	QueryPtr             *SnapmirrorGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                    `xml:"tag"`
}

// SnapmirrorGetIterResponse is a structure to represent a snapmirror-get-iter Response ZAPI object
type SnapmirrorGetIterResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SnapmirrorGetIterResponseResult `xml:"results"`
}

// NewSnapmirrorGetIterResponse is a factory method for creating new instances of SnapmirrorGetIterResponse objects
func NewSnapmirrorGetIterResponse() *SnapmirrorGetIterResponse {
	return &SnapmirrorGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorGetIterResponseResult is a structure to represent a snapmirror-get-iter Response Result ZAPI object
type SnapmirrorGetIterResponseResult struct {
	XMLName           xml.Name                                       `xml:"results"`
	ResultStatusAttr  string                                         `xml:"status,attr"`
	ResultReasonAttr  string                                         `xml:"reason,attr"`
	ResultErrnoAttr   string                                         `xml:"errno,attr"`
	AttributesListPtr *SnapmirrorGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                        `xml:"next-tag"`
	NumRecordsPtr     *int                                           `xml:"num-records"`
}

// NewSnapmirrorGetIterRequest is a factory method for creating new instances of SnapmirrorGetIterRequest objects
func NewSnapmirrorGetIterRequest() *SnapmirrorGetIterRequest {
	return &SnapmirrorGetIterRequest{}
}

// NewSnapmirrorGetIterResponseResult is a factory method for creating new instances of SnapmirrorGetIterResponseResult objects
func NewSnapmirrorGetIterResponseResult() *SnapmirrorGetIterResponseResult {
	return &SnapmirrorGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorGetIterRequest", NewSnapmirrorGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorGetIterRequest) executeWithIteration(zr *ZapiRunner) (*SnapmirrorGetIterResponse, error) {
	combined := NewSnapmirrorGetIterResponse()
	combined.Result.SetAttributesList(SnapmirrorGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for !done {
		n, err := o.executeWithoutIteration(zr)

		if err != nil {
			return nil, err
		}
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
			if combined.Result.AttributesListPtr == nil {
				combined.Result.SetAttributesList(SnapmirrorGetIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done {

			combined.Result.ResultErrnoAttr = n.Result.ResultErrnoAttr
			combined.Result.ResultReasonAttr = n.Result.ResultReasonAttr
			combined.Result.ResultStatusAttr = n.Result.ResultStatusAttr

			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()
			combined.Result.SetNumRecords(len(combinedAttributes))

		}
	}
	return combined, nil
}

// SnapmirrorGetIterRequestDesiredAttributes is a wrapper
type SnapmirrorGetIterRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	SnapmirrorInfoPtr *SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorGetIterRequestDesiredAttributes) SnapmirrorInfo() SnapmirrorInfoType {
	var r SnapmirrorInfoType
	if o.SnapmirrorInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequestDesiredAttributes) SetSnapmirrorInfo(newValue SnapmirrorInfoType) *SnapmirrorGetIterRequestDesiredAttributes {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SnapmirrorGetIterRequest) DesiredAttributes() SnapmirrorGetIterRequestDesiredAttributes {
	var r SnapmirrorGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetDesiredAttributes(newValue SnapmirrorGetIterRequestDesiredAttributes) *SnapmirrorGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// Expand is a 'getter' method
func (o *SnapmirrorGetIterRequest) Expand() bool {
	var r bool
	if o.ExpandPtr == nil {
		return r
	}
	r = *o.ExpandPtr
	return r
}

// SetExpand is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetExpand(newValue bool) *SnapmirrorGetIterRequest {
	o.ExpandPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SnapmirrorGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetMaxRecords(newValue int) *SnapmirrorGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SnapmirrorGetIterRequestQuery is a wrapper
type SnapmirrorGetIterRequestQuery struct {
	XMLName           xml.Name            `xml:"query"`
	SnapmirrorInfoPtr *SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorGetIterRequestQuery) SnapmirrorInfo() SnapmirrorInfoType {
	var r SnapmirrorInfoType
	if o.SnapmirrorInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequestQuery) SetSnapmirrorInfo(newValue SnapmirrorInfoType) *SnapmirrorGetIterRequestQuery {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SnapmirrorGetIterRequest) Query() SnapmirrorGetIterRequestQuery {
	var r SnapmirrorGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetQuery(newValue SnapmirrorGetIterRequestQuery) *SnapmirrorGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SnapmirrorGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterRequest) SetTag(newValue string) *SnapmirrorGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapmirrorGetIterResponseResultAttributesList is a wrapper
type SnapmirrorGetIterResponseResultAttributesList struct {
	XMLName           xml.Name             `xml:"attributes-list"`
	SnapmirrorInfoPtr []SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorGetIterResponseResultAttributesList) SnapmirrorInfo() []SnapmirrorInfoType {
	r := o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResultAttributesList) SetSnapmirrorInfo(newValue []SnapmirrorInfoType) *SnapmirrorGetIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapmirrorGetIterResponseResultAttributesList) values() []SnapmirrorInfoType {
	r := o.SnapmirrorInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResultAttributesList) setValues(newValue []SnapmirrorInfoType) *SnapmirrorGetIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *SnapmirrorGetIterResponseResult) AttributesList() SnapmirrorGetIterResponseResultAttributesList {
	var r SnapmirrorGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetAttributesList(newValue SnapmirrorGetIterResponseResultAttributesList) *SnapmirrorGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SnapmirrorGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetNextTag(newValue string) *SnapmirrorGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *SnapmirrorGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetIterResponseResult) SetNumRecords(newValue int) *SnapmirrorGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
