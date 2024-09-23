// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotGetIterRequest is a structure to represent a snapshot-get-iter Request ZAPI object
type SnapshotGetIterRequest struct {
	XMLName              xml.Name                                 `xml:"snapshot-get-iter"`
	DesiredAttributesPtr *SnapshotGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                     `xml:"max-records"`
	QueryPtr             *SnapshotGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                  `xml:"tag"`
}

// SnapshotGetIterResponse is a structure to represent a snapshot-get-iter Response ZAPI object
type SnapshotGetIterResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          SnapshotGetIterResponseResult `xml:"results"`
}

// NewSnapshotGetIterResponse is a factory method for creating new instances of SnapshotGetIterResponse objects
func NewSnapshotGetIterResponse() *SnapshotGetIterResponse {
	return &SnapshotGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotGetIterResponseResult is a structure to represent a snapshot-get-iter Response Result ZAPI object
type SnapshotGetIterResponseResult struct {
	XMLName           xml.Name                                     `xml:"results"`
	ResultStatusAttr  string                                       `xml:"status,attr"`
	ResultReasonAttr  string                                       `xml:"reason,attr"`
	ResultErrnoAttr   string                                       `xml:"errno,attr"`
	AttributesListPtr *SnapshotGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                      `xml:"next-tag"`
	NumRecordsPtr     *int                                         `xml:"num-records"`
	VolumeErrorsPtr   *SnapshotGetIterResponseResultVolumeErrors   `xml:"volume-errors"`
}

// NewSnapshotGetIterRequest is a factory method for creating new instances of SnapshotGetIterRequest objects
func NewSnapshotGetIterRequest() *SnapshotGetIterRequest {
	return &SnapshotGetIterRequest{}
}

// NewSnapshotGetIterResponseResult is a factory method for creating new instances of SnapshotGetIterResponseResult objects
func NewSnapshotGetIterResponseResult() *SnapshotGetIterResponseResult {
	return &SnapshotGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotGetIterRequest", NewSnapshotGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapshotGetIterRequest) executeWithIteration(zr *ZapiRunner) (*SnapshotGetIterResponse, error) {
	combined := NewSnapshotGetIterResponse()
	combined.Result.SetAttributesList(SnapshotGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(SnapshotGetIterResponseResultAttributesList{})
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

// SnapshotGetIterRequestDesiredAttributes is a wrapper
type SnapshotGetIterRequestDesiredAttributes struct {
	XMLName         xml.Name          `xml:"desired-attributes"`
	SnapshotInfoPtr *SnapshotInfoType `xml:"snapshot-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapshotInfo is a 'getter' method
func (o *SnapshotGetIterRequestDesiredAttributes) SnapshotInfo() SnapshotInfoType {
	var r SnapshotInfoType
	if o.SnapshotInfoPtr == nil {
		return r
	}
	r = *o.SnapshotInfoPtr
	return r
}

// SetSnapshotInfo is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequestDesiredAttributes) SetSnapshotInfo(newValue SnapshotInfoType) *SnapshotGetIterRequestDesiredAttributes {
	o.SnapshotInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SnapshotGetIterRequest) DesiredAttributes() SnapshotGetIterRequestDesiredAttributes {
	var r SnapshotGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetDesiredAttributes(newValue SnapshotGetIterRequestDesiredAttributes) *SnapshotGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SnapshotGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetMaxRecords(newValue int) *SnapshotGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SnapshotGetIterRequestQuery is a wrapper
type SnapshotGetIterRequestQuery struct {
	XMLName         xml.Name          `xml:"query"`
	SnapshotInfoPtr *SnapshotInfoType `xml:"snapshot-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapshotInfo is a 'getter' method
func (o *SnapshotGetIterRequestQuery) SnapshotInfo() SnapshotInfoType {
	var r SnapshotInfoType
	if o.SnapshotInfoPtr == nil {
		return r
	}
	r = *o.SnapshotInfoPtr
	return r
}

// SetSnapshotInfo is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequestQuery) SetSnapshotInfo(newValue SnapshotInfoType) *SnapshotGetIterRequestQuery {
	o.SnapshotInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SnapshotGetIterRequest) Query() SnapshotGetIterRequestQuery {
	var r SnapshotGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetQuery(newValue SnapshotGetIterRequestQuery) *SnapshotGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SnapshotGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterRequest) SetTag(newValue string) *SnapshotGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapshotGetIterResponseResultAttributesList is a wrapper
type SnapshotGetIterResponseResultAttributesList struct {
	XMLName         xml.Name           `xml:"attributes-list"`
	SnapshotInfoPtr []SnapshotInfoType `xml:"snapshot-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapshotInfo is a 'getter' method
func (o *SnapshotGetIterResponseResultAttributesList) SnapshotInfo() []SnapshotInfoType {
	r := o.SnapshotInfoPtr
	return r
}

// SetSnapshotInfo is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResultAttributesList) SetSnapshotInfo(newValue []SnapshotInfoType) *SnapshotGetIterResponseResultAttributesList {
	newSlice := make([]SnapshotInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapshotInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapshotGetIterResponseResultAttributesList) values() []SnapshotInfoType {
	r := o.SnapshotInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResultAttributesList) setValues(newValue []SnapshotInfoType) *SnapshotGetIterResponseResultAttributesList {
	newSlice := make([]SnapshotInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapshotInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *SnapshotGetIterResponseResult) AttributesList() SnapshotGetIterResponseResultAttributesList {
	var r SnapshotGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetAttributesList(newValue SnapshotGetIterResponseResultAttributesList) *SnapshotGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SnapshotGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetNextTag(newValue string) *SnapshotGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *SnapshotGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetNumRecords(newValue int) *SnapshotGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}

// SnapshotGetIterResponseResultVolumeErrors is a wrapper
type SnapshotGetIterResponseResultVolumeErrors struct {
	XMLName        xml.Name          `xml:"volume-errors"`
	VolumeErrorPtr []VolumeErrorType `xml:"volume-error"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotGetIterResponseResultVolumeErrors) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeError is a 'getter' method
func (o *SnapshotGetIterResponseResultVolumeErrors) VolumeError() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// SetVolumeError is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResultVolumeErrors) SetVolumeError(newValue []VolumeErrorType) *SnapshotGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapshotGetIterResponseResultVolumeErrors) values() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResultVolumeErrors) setValues(newValue []VolumeErrorType) *SnapshotGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// VolumeErrors is a 'getter' method
func (o *SnapshotGetIterResponseResult) VolumeErrors() SnapshotGetIterResponseResultVolumeErrors {
	var r SnapshotGetIterResponseResultVolumeErrors
	if o.VolumeErrorsPtr == nil {
		return r
	}
	r = *o.VolumeErrorsPtr
	return r
}

// SetVolumeErrors is a fluent style 'setter' method that can be chained
func (o *SnapshotGetIterResponseResult) SetVolumeErrors(newValue SnapshotGetIterResponseResultVolumeErrors) *SnapshotGetIterResponseResult {
	o.VolumeErrorsPtr = &newValue
	return o
}
