// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// CifsShareGetIterRequest is a structure to represent a cifs-share-get-iter Request ZAPI object
type CifsShareGetIterRequest struct {
	XMLName              xml.Name                                  `xml:"cifs-share-get-iter"`
	DesiredAttributesPtr *CifsShareGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                      `xml:"max-records"`
	QueryPtr             *CifsShareGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                   `xml:"tag"`
}

// CifsShareGetIterResponse is a structure to represent a cifs-share-get-iter Response ZAPI object
type CifsShareGetIterResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          CifsShareGetIterResponseResult `xml:"results"`
}

// NewCifsShareGetIterResponse is a factory method for creating new instances of CifsShareGetIterResponse objects
func NewCifsShareGetIterResponse() *CifsShareGetIterResponse {
	return &CifsShareGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CifsShareGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CifsShareGetIterResponseResult is a structure to represent a cifs-share-get-iter Response Result ZAPI object
type CifsShareGetIterResponseResult struct {
	XMLName           xml.Name                                      `xml:"results"`
	ResultStatusAttr  string                                        `xml:"status,attr"`
	ResultReasonAttr  string                                        `xml:"reason,attr"`
	ResultErrnoAttr   string                                        `xml:"errno,attr"`
	AttributesListPtr *CifsShareGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                       `xml:"next-tag"`
	NumRecordsPtr     *int                                          `xml:"num-records"`
}

// NewCifsShareGetIterRequest is a factory method for creating new instances of CifsShareGetIterRequest objects
func NewCifsShareGetIterRequest() *CifsShareGetIterRequest {
	return &CifsShareGetIterRequest{}
}

// NewCifsShareGetIterResponseResult is a factory method for creating new instances of CifsShareGetIterResponseResult objects
func NewCifsShareGetIterResponseResult() *CifsShareGetIterResponseResult {
	return &CifsShareGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CifsShareGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*CifsShareGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*CifsShareGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "CifsShareGetIterRequest", NewCifsShareGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CifsShareGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *CifsShareGetIterRequest) executeWithIteration(zr *ZapiRunner) (*CifsShareGetIterResponse, error) {
	combined := NewCifsShareGetIterResponse()
	combined.Result.SetAttributesList(CifsShareGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(CifsShareGetIterResponseResultAttributesList{})
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

// CifsShareGetIterRequestDesiredAttributes is a wrapper
type CifsShareGetIterRequestDesiredAttributes struct {
	XMLName      xml.Name       `xml:"desired-attributes"`
	CifsSharePtr *CifsShareType `xml:"cifs-share"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// CifsShare is a 'getter' method
func (o *CifsShareGetIterRequestDesiredAttributes) CifsShare() CifsShareType {
	var r CifsShareType
	if o.CifsSharePtr == nil {
		return r
	}
	r = *o.CifsSharePtr
	return r
}

// SetCifsShare is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequestDesiredAttributes) SetCifsShare(newValue CifsShareType) *CifsShareGetIterRequestDesiredAttributes {
	o.CifsSharePtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *CifsShareGetIterRequest) DesiredAttributes() CifsShareGetIterRequestDesiredAttributes {
	var r CifsShareGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequest) SetDesiredAttributes(newValue CifsShareGetIterRequestDesiredAttributes) *CifsShareGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *CifsShareGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequest) SetMaxRecords(newValue int) *CifsShareGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// CifsShareGetIterRequestQuery is a wrapper
type CifsShareGetIterRequestQuery struct {
	XMLName      xml.Name       `xml:"query"`
	CifsSharePtr *CifsShareType `xml:"cifs-share"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// CifsShare is a 'getter' method
func (o *CifsShareGetIterRequestQuery) CifsShare() CifsShareType {
	var r CifsShareType
	if o.CifsSharePtr == nil {
		return r
	}
	r = *o.CifsSharePtr
	return r
}

// SetCifsShare is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequestQuery) SetCifsShare(newValue CifsShareType) *CifsShareGetIterRequestQuery {
	o.CifsSharePtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *CifsShareGetIterRequest) Query() CifsShareGetIterRequestQuery {
	var r CifsShareGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequest) SetQuery(newValue CifsShareGetIterRequestQuery) *CifsShareGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *CifsShareGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterRequest) SetTag(newValue string) *CifsShareGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// CifsShareGetIterResponseResultAttributesList is a wrapper
type CifsShareGetIterResponseResultAttributesList struct {
	XMLName      xml.Name        `xml:"attributes-list"`
	CifsSharePtr []CifsShareType `xml:"cifs-share"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// CifsShare is a 'getter' method
func (o *CifsShareGetIterResponseResultAttributesList) CifsShare() []CifsShareType {
	r := o.CifsSharePtr
	return r
}

// SetCifsShare is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterResponseResultAttributesList) SetCifsShare(newValue []CifsShareType) *CifsShareGetIterResponseResultAttributesList {
	newSlice := make([]CifsShareType, len(newValue))
	copy(newSlice, newValue)
	o.CifsSharePtr = newSlice
	return o
}

// values is a 'getter' method
func (o *CifsShareGetIterResponseResultAttributesList) values() []CifsShareType {
	r := o.CifsSharePtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterResponseResultAttributesList) setValues(newValue []CifsShareType) *CifsShareGetIterResponseResultAttributesList {
	newSlice := make([]CifsShareType, len(newValue))
	copy(newSlice, newValue)
	o.CifsSharePtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *CifsShareGetIterResponseResult) AttributesList() CifsShareGetIterResponseResultAttributesList {
	var r CifsShareGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterResponseResult) SetAttributesList(newValue CifsShareGetIterResponseResultAttributesList) *CifsShareGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *CifsShareGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterResponseResult) SetNextTag(newValue string) *CifsShareGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *CifsShareGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *CifsShareGetIterResponseResult) SetNumRecords(newValue int) *CifsShareGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
