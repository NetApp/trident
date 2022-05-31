// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// QuotaListEntriesIterRequest is a structure to represent a quota-list-entries-iter Request ZAPI object
type QuotaListEntriesIterRequest struct {
	XMLName              xml.Name                                      `xml:"quota-list-entries-iter"`
	DesiredAttributesPtr *QuotaListEntriesIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                          `xml:"max-records"`
	QueryPtr             *QuotaListEntriesIterRequestQuery             `xml:"query"`
	TagPtr               *string                                       `xml:"tag"`
}

// QuotaListEntriesIterResponse is a structure to represent a quota-list-entries-iter Response ZAPI object
type QuotaListEntriesIterResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          QuotaListEntriesIterResponseResult `xml:"results"`
}

// NewQuotaListEntriesIterResponse is a factory method for creating new instances of QuotaListEntriesIterResponse objects
func NewQuotaListEntriesIterResponse() *QuotaListEntriesIterResponse {
	return &QuotaListEntriesIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QuotaListEntriesIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QuotaListEntriesIterResponseResult is a structure to represent a quota-list-entries-iter Response Result ZAPI object
type QuotaListEntriesIterResponseResult struct {
	XMLName           xml.Name                                          `xml:"results"`
	ResultStatusAttr  string                                            `xml:"status,attr"`
	ResultReasonAttr  string                                            `xml:"reason,attr"`
	ResultErrnoAttr   string                                            `xml:"errno,attr"`
	AttributesListPtr *QuotaListEntriesIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                           `xml:"next-tag"`
	NumRecordsPtr     *int                                              `xml:"num-records"`
}

// NewQuotaListEntriesIterRequest is a factory method for creating new instances of QuotaListEntriesIterRequest objects
func NewQuotaListEntriesIterRequest() *QuotaListEntriesIterRequest {
	return &QuotaListEntriesIterRequest{}
}

// NewQuotaListEntriesIterResponseResult is a factory method for creating new instances of QuotaListEntriesIterResponseResult objects
func NewQuotaListEntriesIterResponseResult() *QuotaListEntriesIterResponseResult {
	return &QuotaListEntriesIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaListEntriesIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QuotaListEntriesIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaListEntriesIterRequest) ExecuteUsing(zr *ZapiRunner) (*QuotaListEntriesIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaListEntriesIterRequest) executeWithoutIteration(zr *ZapiRunner) (*QuotaListEntriesIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "QuotaListEntriesIterRequest", NewQuotaListEntriesIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QuotaListEntriesIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QuotaListEntriesIterRequest) executeWithIteration(zr *ZapiRunner) (*QuotaListEntriesIterResponse, error) {
	combined := NewQuotaListEntriesIterResponse()
	combined.Result.SetAttributesList(QuotaListEntriesIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(QuotaListEntriesIterResponseResultAttributesList{})
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

// QuotaListEntriesIterRequestDesiredAttributes is a wrapper
type QuotaListEntriesIterRequestDesiredAttributes struct {
	XMLName       xml.Name        `xml:"desired-attributes"`
	QuotaEntryPtr *QuotaEntryType `xml:"quota-entry"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// QuotaEntry is a 'getter' method
func (o *QuotaListEntriesIterRequestDesiredAttributes) QuotaEntry() QuotaEntryType {
	var r QuotaEntryType
	if o.QuotaEntryPtr == nil {
		return r
	}
	r = *o.QuotaEntryPtr
	return r
}

// SetQuotaEntry is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequestDesiredAttributes) SetQuotaEntry(newValue QuotaEntryType) *QuotaListEntriesIterRequestDesiredAttributes {
	o.QuotaEntryPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *QuotaListEntriesIterRequest) DesiredAttributes() QuotaListEntriesIterRequestDesiredAttributes {
	var r QuotaListEntriesIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequest) SetDesiredAttributes(newValue QuotaListEntriesIterRequestDesiredAttributes) *QuotaListEntriesIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *QuotaListEntriesIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequest) SetMaxRecords(newValue int) *QuotaListEntriesIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// QuotaListEntriesIterRequestQuery is a wrapper
type QuotaListEntriesIterRequestQuery struct {
	XMLName       xml.Name        `xml:"query"`
	QuotaEntryPtr *QuotaEntryType `xml:"quota-entry"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// QuotaEntry is a 'getter' method
func (o *QuotaListEntriesIterRequestQuery) QuotaEntry() QuotaEntryType {
	var r QuotaEntryType
	if o.QuotaEntryPtr == nil {
		return r
	}
	r = *o.QuotaEntryPtr
	return r
}

// SetQuotaEntry is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequestQuery) SetQuotaEntry(newValue QuotaEntryType) *QuotaListEntriesIterRequestQuery {
	o.QuotaEntryPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *QuotaListEntriesIterRequest) Query() QuotaListEntriesIterRequestQuery {
	var r QuotaListEntriesIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequest) SetQuery(newValue QuotaListEntriesIterRequestQuery) *QuotaListEntriesIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *QuotaListEntriesIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterRequest) SetTag(newValue string) *QuotaListEntriesIterRequest {
	o.TagPtr = &newValue
	return o
}

// QuotaListEntriesIterResponseResultAttributesList is a wrapper
type QuotaListEntriesIterResponseResultAttributesList struct {
	XMLName       xml.Name         `xml:"attributes-list"`
	QuotaEntryPtr []QuotaEntryType `xml:"quota-entry"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaListEntriesIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// QuotaEntry is a 'getter' method
func (o *QuotaListEntriesIterResponseResultAttributesList) QuotaEntry() []QuotaEntryType {
	r := o.QuotaEntryPtr
	return r
}

// SetQuotaEntry is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterResponseResultAttributesList) SetQuotaEntry(newValue []QuotaEntryType) *QuotaListEntriesIterResponseResultAttributesList {
	newSlice := make([]QuotaEntryType, len(newValue))
	copy(newSlice, newValue)
	o.QuotaEntryPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *QuotaListEntriesIterResponseResultAttributesList) values() []QuotaEntryType {
	r := o.QuotaEntryPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterResponseResultAttributesList) setValues(newValue []QuotaEntryType) *QuotaListEntriesIterResponseResultAttributesList {
	newSlice := make([]QuotaEntryType, len(newValue))
	copy(newSlice, newValue)
	o.QuotaEntryPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *QuotaListEntriesIterResponseResult) AttributesList() QuotaListEntriesIterResponseResultAttributesList {
	var r QuotaListEntriesIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterResponseResult) SetAttributesList(newValue QuotaListEntriesIterResponseResultAttributesList) *QuotaListEntriesIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *QuotaListEntriesIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterResponseResult) SetNextTag(newValue string) *QuotaListEntriesIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *QuotaListEntriesIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *QuotaListEntriesIterResponseResult) SetNumRecords(newValue int) *QuotaListEntriesIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
