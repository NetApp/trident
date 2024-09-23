// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeNamespaceGetIterRequest is a structure to represent a nvme-namespace-get-iter Request ZAPI object
type NvmeNamespaceGetIterRequest struct {
	XMLName              xml.Name                                      `xml:"nvme-namespace-get-iter"`
	DesiredAttributesPtr *NvmeNamespaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                          `xml:"max-records"`
	QueryPtr             *NvmeNamespaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                       `xml:"tag"`
}

// NvmeNamespaceGetIterResponse is a structure to represent a nvme-namespace-get-iter Response ZAPI object
type NvmeNamespaceGetIterResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          NvmeNamespaceGetIterResponseResult `xml:"results"`
}

// NewNvmeNamespaceGetIterResponse is a factory method for creating new instances of NvmeNamespaceGetIterResponse objects
func NewNvmeNamespaceGetIterResponse() *NvmeNamespaceGetIterResponse {
	return &NvmeNamespaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeNamespaceGetIterResponseResult is a structure to represent a nvme-namespace-get-iter Response Result ZAPI object
type NvmeNamespaceGetIterResponseResult struct {
	XMLName           xml.Name                                          `xml:"results"`
	ResultStatusAttr  string                                            `xml:"status,attr"`
	ResultReasonAttr  string                                            `xml:"reason,attr"`
	ResultErrnoAttr   string                                            `xml:"errno,attr"`
	AttributesListPtr *NvmeNamespaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                           `xml:"next-tag"`
	NumRecordsPtr     *int                                              `xml:"num-records"`
	VolumeErrorsPtr   *NvmeNamespaceGetIterResponseResultVolumeErrors   `xml:"volume-errors"`
}

// NewNvmeNamespaceGetIterRequest is a factory method for creating new instances of NvmeNamespaceGetIterRequest objects
func NewNvmeNamespaceGetIterRequest() *NvmeNamespaceGetIterRequest {
	return &NvmeNamespaceGetIterRequest{}
}

// NewNvmeNamespaceGetIterResponseResult is a factory method for creating new instances of NvmeNamespaceGetIterResponseResult objects
func NewNvmeNamespaceGetIterResponseResult() *NvmeNamespaceGetIterResponseResult {
	return &NvmeNamespaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeNamespaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeNamespaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeNamespaceGetIterRequest", NewNvmeNamespaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeNamespaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeNamespaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeNamespaceGetIterResponse, error) {
	combined := NewNvmeNamespaceGetIterResponse()
	combined.Result.SetAttributesList(NvmeNamespaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeNamespaceGetIterResponseResultAttributesList{})
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

// NvmeNamespaceGetIterRequestDesiredAttributes is a wrapper
type NvmeNamespaceGetIterRequestDesiredAttributes struct {
	XMLName              xml.Name               `xml:"desired-attributes"`
	NvmeNamespaceInfoPtr *NvmeNamespaceInfoType `xml:"nvme-namespace-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeNamespaceInfo is a 'getter' method
func (o *NvmeNamespaceGetIterRequestDesiredAttributes) NvmeNamespaceInfo() NvmeNamespaceInfoType {
	var r NvmeNamespaceInfoType
	if o.NvmeNamespaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeNamespaceInfoPtr
	return r
}

// SetNvmeNamespaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequestDesiredAttributes) SetNvmeNamespaceInfo(newValue NvmeNamespaceInfoType) *NvmeNamespaceGetIterRequestDesiredAttributes {
	o.NvmeNamespaceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeNamespaceGetIterRequest) DesiredAttributes() NvmeNamespaceGetIterRequestDesiredAttributes {
	var r NvmeNamespaceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequest) SetDesiredAttributes(newValue NvmeNamespaceGetIterRequestDesiredAttributes) *NvmeNamespaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeNamespaceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequest) SetMaxRecords(newValue int) *NvmeNamespaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeNamespaceGetIterRequestQuery is a wrapper
type NvmeNamespaceGetIterRequestQuery struct {
	XMLName              xml.Name               `xml:"query"`
	NvmeNamespaceInfoPtr *NvmeNamespaceInfoType `xml:"nvme-namespace-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeNamespaceInfo is a 'getter' method
func (o *NvmeNamespaceGetIterRequestQuery) NvmeNamespaceInfo() NvmeNamespaceInfoType {
	var r NvmeNamespaceInfoType
	if o.NvmeNamespaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeNamespaceInfoPtr
	return r
}

// SetNvmeNamespaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequestQuery) SetNvmeNamespaceInfo(newValue NvmeNamespaceInfoType) *NvmeNamespaceGetIterRequestQuery {
	o.NvmeNamespaceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeNamespaceGetIterRequest) Query() NvmeNamespaceGetIterRequestQuery {
	var r NvmeNamespaceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequest) SetQuery(newValue NvmeNamespaceGetIterRequestQuery) *NvmeNamespaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeNamespaceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterRequest) SetTag(newValue string) *NvmeNamespaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeNamespaceGetIterResponseResultAttributesList is a wrapper
type NvmeNamespaceGetIterResponseResultAttributesList struct {
	XMLName              xml.Name                `xml:"attributes-list"`
	NvmeNamespaceInfoPtr []NvmeNamespaceInfoType `xml:"nvme-namespace-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeNamespaceInfo is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResultAttributesList) NvmeNamespaceInfo() []NvmeNamespaceInfoType {
	r := o.NvmeNamespaceInfoPtr
	return r
}

// SetNvmeNamespaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResultAttributesList) SetNvmeNamespaceInfo(newValue []NvmeNamespaceInfoType) *NvmeNamespaceGetIterResponseResultAttributesList {
	newSlice := make([]NvmeNamespaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeNamespaceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResultAttributesList) values() []NvmeNamespaceInfoType {
	r := o.NvmeNamespaceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResultAttributesList) setValues(newValue []NvmeNamespaceInfoType) *NvmeNamespaceGetIterResponseResultAttributesList {
	newSlice := make([]NvmeNamespaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeNamespaceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResult) AttributesList() NvmeNamespaceGetIterResponseResultAttributesList {
	var r NvmeNamespaceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResult) SetAttributesList(newValue NvmeNamespaceGetIterResponseResultAttributesList) *NvmeNamespaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResult) SetNextTag(newValue string) *NvmeNamespaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResult) SetNumRecords(newValue int) *NvmeNamespaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}

// NvmeNamespaceGetIterResponseResultVolumeErrors is a wrapper
type NvmeNamespaceGetIterResponseResultVolumeErrors struct {
	XMLName        xml.Name          `xml:"volume-errors"`
	VolumeErrorPtr []VolumeErrorType `xml:"volume-error"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceGetIterResponseResultVolumeErrors) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeError is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResultVolumeErrors) VolumeError() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// SetVolumeError is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResultVolumeErrors) SetVolumeError(newValue []VolumeErrorType) *NvmeNamespaceGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResultVolumeErrors) values() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResultVolumeErrors) setValues(newValue []VolumeErrorType) *NvmeNamespaceGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// VolumeErrors is a 'getter' method
func (o *NvmeNamespaceGetIterResponseResult) VolumeErrors() NvmeNamespaceGetIterResponseResultVolumeErrors {
	var r NvmeNamespaceGetIterResponseResultVolumeErrors
	if o.VolumeErrorsPtr == nil {
		return r
	}
	r = *o.VolumeErrorsPtr
	return r
}

// SetVolumeErrors is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceGetIterResponseResult) SetVolumeErrors(newValue NvmeNamespaceGetIterResponseResultVolumeErrors) *NvmeNamespaceGetIterResponseResult {
	o.VolumeErrorsPtr = &newValue
	return o
}
