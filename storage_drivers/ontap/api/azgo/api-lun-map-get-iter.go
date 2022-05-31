// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// LunMapGetIterRequest is a structure to represent a lun-map-get-iter Request ZAPI object
type LunMapGetIterRequest struct {
	XMLName              xml.Name                               `xml:"lun-map-get-iter"`
	DesiredAttributesPtr *LunMapGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                   `xml:"max-records"`
	QueryPtr             *LunMapGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                `xml:"tag"`
}

// LunMapGetIterResponse is a structure to represent a lun-map-get-iter Response ZAPI object
type LunMapGetIterResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          LunMapGetIterResponseResult `xml:"results"`
}

// NewLunMapGetIterResponse is a factory method for creating new instances of LunMapGetIterResponse objects
func NewLunMapGetIterResponse() *LunMapGetIterResponse {
	return &LunMapGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunMapGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunMapGetIterResponseResult is a structure to represent a lun-map-get-iter Response Result ZAPI object
type LunMapGetIterResponseResult struct {
	XMLName           xml.Name                                   `xml:"results"`
	ResultStatusAttr  string                                     `xml:"status,attr"`
	ResultReasonAttr  string                                     `xml:"reason,attr"`
	ResultErrnoAttr   string                                     `xml:"errno,attr"`
	AttributesListPtr *LunMapGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                    `xml:"next-tag"`
	NumRecordsPtr     *int                                       `xml:"num-records"`
	VolumeErrorsPtr   *LunMapGetIterResponseResultVolumeErrors   `xml:"volume-errors"`
}

// NewLunMapGetIterRequest is a factory method for creating new instances of LunMapGetIterRequest objects
func NewLunMapGetIterRequest() *LunMapGetIterRequest {
	return &LunMapGetIterRequest{}
}

// NewLunMapGetIterResponseResult is a factory method for creating new instances of LunMapGetIterResponseResult objects
func NewLunMapGetIterResponseResult() *LunMapGetIterResponseResult {
	return &LunMapGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunMapGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunMapGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*LunMapGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*LunMapGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunMapGetIterRequest", NewLunMapGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunMapGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunMapGetIterRequest) executeWithIteration(zr *ZapiRunner) (*LunMapGetIterResponse, error) {
	combined := NewLunMapGetIterResponse()
	combined.Result.SetAttributesList(LunMapGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(LunMapGetIterResponseResultAttributesList{})
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

// LunMapGetIterRequestDesiredAttributes is a wrapper
type LunMapGetIterRequestDesiredAttributes struct {
	XMLName       xml.Name        `xml:"desired-attributes"`
	LunMapInfoPtr *LunMapInfoType `xml:"lun-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunMapInfo is a 'getter' method
func (o *LunMapGetIterRequestDesiredAttributes) LunMapInfo() LunMapInfoType {
	var r LunMapInfoType
	if o.LunMapInfoPtr == nil {
		return r
	}
	r = *o.LunMapInfoPtr
	return r
}

// SetLunMapInfo is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequestDesiredAttributes) SetLunMapInfo(newValue LunMapInfoType) *LunMapGetIterRequestDesiredAttributes {
	o.LunMapInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *LunMapGetIterRequest) DesiredAttributes() LunMapGetIterRequestDesiredAttributes {
	var r LunMapGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequest) SetDesiredAttributes(newValue LunMapGetIterRequestDesiredAttributes) *LunMapGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *LunMapGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequest) SetMaxRecords(newValue int) *LunMapGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// LunMapGetIterRequestQuery is a wrapper
type LunMapGetIterRequestQuery struct {
	XMLName       xml.Name        `xml:"query"`
	LunMapInfoPtr *LunMapInfoType `xml:"lun-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunMapInfo is a 'getter' method
func (o *LunMapGetIterRequestQuery) LunMapInfo() LunMapInfoType {
	var r LunMapInfoType
	if o.LunMapInfoPtr == nil {
		return r
	}
	r = *o.LunMapInfoPtr
	return r
}

// SetLunMapInfo is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequestQuery) SetLunMapInfo(newValue LunMapInfoType) *LunMapGetIterRequestQuery {
	o.LunMapInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *LunMapGetIterRequest) Query() LunMapGetIterRequestQuery {
	var r LunMapGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequest) SetQuery(newValue LunMapGetIterRequestQuery) *LunMapGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *LunMapGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterRequest) SetTag(newValue string) *LunMapGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// LunMapGetIterResponseResultAttributesList is a wrapper
type LunMapGetIterResponseResultAttributesList struct {
	XMLName       xml.Name         `xml:"attributes-list"`
	LunMapInfoPtr []LunMapInfoType `xml:"lun-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunMapInfo is a 'getter' method
func (o *LunMapGetIterResponseResultAttributesList) LunMapInfo() []LunMapInfoType {
	r := o.LunMapInfoPtr
	return r
}

// SetLunMapInfo is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResultAttributesList) SetLunMapInfo(newValue []LunMapInfoType) *LunMapGetIterResponseResultAttributesList {
	newSlice := make([]LunMapInfoType, len(newValue))
	copy(newSlice, newValue)
	o.LunMapInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *LunMapGetIterResponseResultAttributesList) values() []LunMapInfoType {
	r := o.LunMapInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResultAttributesList) setValues(newValue []LunMapInfoType) *LunMapGetIterResponseResultAttributesList {
	newSlice := make([]LunMapInfoType, len(newValue))
	copy(newSlice, newValue)
	o.LunMapInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *LunMapGetIterResponseResult) AttributesList() LunMapGetIterResponseResultAttributesList {
	var r LunMapGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResult) SetAttributesList(newValue LunMapGetIterResponseResultAttributesList) *LunMapGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *LunMapGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResult) SetNextTag(newValue string) *LunMapGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *LunMapGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResult) SetNumRecords(newValue int) *LunMapGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}

// LunMapGetIterResponseResultVolumeErrors is a wrapper
type LunMapGetIterResponseResultVolumeErrors struct {
	XMLName        xml.Name          `xml:"volume-errors"`
	VolumeErrorPtr []VolumeErrorType `xml:"volume-error"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapGetIterResponseResultVolumeErrors) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeError is a 'getter' method
func (o *LunMapGetIterResponseResultVolumeErrors) VolumeError() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// SetVolumeError is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResultVolumeErrors) SetVolumeError(newValue []VolumeErrorType) *LunMapGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *LunMapGetIterResponseResultVolumeErrors) values() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResultVolumeErrors) setValues(newValue []VolumeErrorType) *LunMapGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// VolumeErrors is a 'getter' method
func (o *LunMapGetIterResponseResult) VolumeErrors() LunMapGetIterResponseResultVolumeErrors {
	var r LunMapGetIterResponseResultVolumeErrors
	if o.VolumeErrorsPtr == nil {
		return r
	}
	r = *o.VolumeErrorsPtr
	return r
}

// SetVolumeErrors is a fluent style 'setter' method that can be chained
func (o *LunMapGetIterResponseResult) SetVolumeErrors(newValue LunMapGetIterResponseResultVolumeErrors) *LunMapGetIterResponseResult {
	o.VolumeErrorsPtr = &newValue
	return o
}
