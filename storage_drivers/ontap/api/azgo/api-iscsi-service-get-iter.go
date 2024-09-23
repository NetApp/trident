// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiServiceGetIterRequest is a structure to represent a iscsi-service-get-iter Request ZAPI object
type IscsiServiceGetIterRequest struct {
	XMLName              xml.Name                                     `xml:"iscsi-service-get-iter"`
	DesiredAttributesPtr *IscsiServiceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                         `xml:"max-records"`
	QueryPtr             *IscsiServiceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                      `xml:"tag"`
}

// IscsiServiceGetIterResponse is a structure to represent a iscsi-service-get-iter Response ZAPI object
type IscsiServiceGetIterResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          IscsiServiceGetIterResponseResult `xml:"results"`
}

// NewIscsiServiceGetIterResponse is a factory method for creating new instances of IscsiServiceGetIterResponse objects
func NewIscsiServiceGetIterResponse() *IscsiServiceGetIterResponse {
	return &IscsiServiceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiServiceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiServiceGetIterResponseResult is a structure to represent a iscsi-service-get-iter Response Result ZAPI object
type IscsiServiceGetIterResponseResult struct {
	XMLName           xml.Name                                         `xml:"results"`
	ResultStatusAttr  string                                           `xml:"status,attr"`
	ResultReasonAttr  string                                           `xml:"reason,attr"`
	ResultErrnoAttr   string                                           `xml:"errno,attr"`
	AttributesListPtr *IscsiServiceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                          `xml:"next-tag"`
	NumRecordsPtr     *int                                             `xml:"num-records"`
}

// NewIscsiServiceGetIterRequest is a factory method for creating new instances of IscsiServiceGetIterRequest objects
func NewIscsiServiceGetIterRequest() *IscsiServiceGetIterRequest {
	return &IscsiServiceGetIterRequest{}
}

// NewIscsiServiceGetIterResponseResult is a factory method for creating new instances of IscsiServiceGetIterResponseResult objects
func NewIscsiServiceGetIterResponseResult() *IscsiServiceGetIterResponseResult {
	return &IscsiServiceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiServiceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiServiceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiServiceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiServiceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiServiceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiServiceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiServiceGetIterRequest", NewIscsiServiceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiServiceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiServiceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*IscsiServiceGetIterResponse, error) {
	combined := NewIscsiServiceGetIterResponse()
	combined.Result.SetAttributesList(IscsiServiceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(IscsiServiceGetIterResponseResultAttributesList{})
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

// IscsiServiceGetIterRequestDesiredAttributes is a wrapper
type IscsiServiceGetIterRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	IscsiServiceInfoPtr *IscsiServiceInfoType `xml:"iscsi-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiServiceInfo is a 'getter' method
func (o *IscsiServiceGetIterRequestDesiredAttributes) IscsiServiceInfo() IscsiServiceInfoType {
	var r IscsiServiceInfoType
	if o.IscsiServiceInfoPtr == nil {
		return r
	}
	r = *o.IscsiServiceInfoPtr
	return r
}

// SetIscsiServiceInfo is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequestDesiredAttributes) SetIscsiServiceInfo(newValue IscsiServiceInfoType) *IscsiServiceGetIterRequestDesiredAttributes {
	o.IscsiServiceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *IscsiServiceGetIterRequest) DesiredAttributes() IscsiServiceGetIterRequestDesiredAttributes {
	var r IscsiServiceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequest) SetDesiredAttributes(newValue IscsiServiceGetIterRequestDesiredAttributes) *IscsiServiceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *IscsiServiceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequest) SetMaxRecords(newValue int) *IscsiServiceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// IscsiServiceGetIterRequestQuery is a wrapper
type IscsiServiceGetIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	IscsiServiceInfoPtr *IscsiServiceInfoType `xml:"iscsi-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiServiceInfo is a 'getter' method
func (o *IscsiServiceGetIterRequestQuery) IscsiServiceInfo() IscsiServiceInfoType {
	var r IscsiServiceInfoType
	if o.IscsiServiceInfoPtr == nil {
		return r
	}
	r = *o.IscsiServiceInfoPtr
	return r
}

// SetIscsiServiceInfo is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequestQuery) SetIscsiServiceInfo(newValue IscsiServiceInfoType) *IscsiServiceGetIterRequestQuery {
	o.IscsiServiceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *IscsiServiceGetIterRequest) Query() IscsiServiceGetIterRequestQuery {
	var r IscsiServiceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequest) SetQuery(newValue IscsiServiceGetIterRequestQuery) *IscsiServiceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *IscsiServiceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterRequest) SetTag(newValue string) *IscsiServiceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// IscsiServiceGetIterResponseResultAttributesList is a wrapper
type IscsiServiceGetIterResponseResultAttributesList struct {
	XMLName             xml.Name               `xml:"attributes-list"`
	IscsiServiceInfoPtr []IscsiServiceInfoType `xml:"iscsi-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiServiceInfo is a 'getter' method
func (o *IscsiServiceGetIterResponseResultAttributesList) IscsiServiceInfo() []IscsiServiceInfoType {
	r := o.IscsiServiceInfoPtr
	return r
}

// SetIscsiServiceInfo is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterResponseResultAttributesList) SetIscsiServiceInfo(newValue []IscsiServiceInfoType) *IscsiServiceGetIterResponseResultAttributesList {
	newSlice := make([]IscsiServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiServiceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IscsiServiceGetIterResponseResultAttributesList) values() []IscsiServiceInfoType {
	r := o.IscsiServiceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterResponseResultAttributesList) setValues(newValue []IscsiServiceInfoType) *IscsiServiceGetIterResponseResultAttributesList {
	newSlice := make([]IscsiServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiServiceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *IscsiServiceGetIterResponseResult) AttributesList() IscsiServiceGetIterResponseResultAttributesList {
	var r IscsiServiceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterResponseResult) SetAttributesList(newValue IscsiServiceGetIterResponseResultAttributesList) *IscsiServiceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *IscsiServiceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterResponseResult) SetNextTag(newValue string) *IscsiServiceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *IscsiServiceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *IscsiServiceGetIterResponseResult) SetNumRecords(newValue int) *IscsiServiceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
