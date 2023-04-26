// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemGetIterRequest is a structure to represent a nvme-subsystem-get-iter Request ZAPI object
type NvmeSubsystemGetIterRequest struct {
	XMLName              xml.Name                                      `xml:"nvme-subsystem-get-iter"`
	DesiredAttributesPtr *NvmeSubsystemGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                          `xml:"max-records"`
	QueryPtr             *NvmeSubsystemGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                       `xml:"tag"`
}

// NvmeSubsystemGetIterResponse is a structure to represent a nvme-subsystem-get-iter Response ZAPI object
type NvmeSubsystemGetIterResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          NvmeSubsystemGetIterResponseResult `xml:"results"`
}

// NewNvmeSubsystemGetIterResponse is a factory method for creating new instances of NvmeSubsystemGetIterResponse objects
func NewNvmeSubsystemGetIterResponse() *NvmeSubsystemGetIterResponse {
	return &NvmeSubsystemGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemGetIterResponseResult is a structure to represent a nvme-subsystem-get-iter Response Result ZAPI object
type NvmeSubsystemGetIterResponseResult struct {
	XMLName           xml.Name                                          `xml:"results"`
	ResultStatusAttr  string                                            `xml:"status,attr"`
	ResultReasonAttr  string                                            `xml:"reason,attr"`
	ResultErrnoAttr   string                                            `xml:"errno,attr"`
	AttributesListPtr *NvmeSubsystemGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                           `xml:"next-tag"`
	NumRecordsPtr     *int                                              `xml:"num-records"`
}

// NewNvmeSubsystemGetIterRequest is a factory method for creating new instances of NvmeSubsystemGetIterRequest objects
func NewNvmeSubsystemGetIterRequest() *NvmeSubsystemGetIterRequest {
	return &NvmeSubsystemGetIterRequest{}
}

// NewNvmeSubsystemGetIterResponseResult is a factory method for creating new instances of NvmeSubsystemGetIterResponseResult objects
func NewNvmeSubsystemGetIterResponseResult() *NvmeSubsystemGetIterResponseResult {
	return &NvmeSubsystemGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemGetIterRequest", NewNvmeSubsystemGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeSubsystemGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeSubsystemGetIterResponse, error) {
	combined := NewNvmeSubsystemGetIterResponse()
	combined.Result.SetAttributesList(NvmeSubsystemGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeSubsystemGetIterResponseResultAttributesList{})
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

// NvmeSubsystemGetIterRequestDesiredAttributes is a wrapper
type NvmeSubsystemGetIterRequestDesiredAttributes struct {
	XMLName              xml.Name               `xml:"desired-attributes"`
	NvmeSubsystemInfoPtr *NvmeSubsystemInfoType `xml:"nvme-subsystem-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeSubsystemInfo is a 'getter' method
func (o *NvmeSubsystemGetIterRequestDesiredAttributes) NvmeSubsystemInfo() NvmeSubsystemInfoType {
	var r NvmeSubsystemInfoType
	if o.NvmeSubsystemInfoPtr == nil {
		return r
	}
	r = *o.NvmeSubsystemInfoPtr
	return r
}

// SetNvmeSubsystemInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequestDesiredAttributes) SetNvmeSubsystemInfo(newValue NvmeSubsystemInfoType) *NvmeSubsystemGetIterRequestDesiredAttributes {
	o.NvmeSubsystemInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeSubsystemGetIterRequest) DesiredAttributes() NvmeSubsystemGetIterRequestDesiredAttributes {
	var r NvmeSubsystemGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequest) SetDesiredAttributes(newValue NvmeSubsystemGetIterRequestDesiredAttributes) *NvmeSubsystemGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeSubsystemGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequest) SetMaxRecords(newValue int) *NvmeSubsystemGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeSubsystemGetIterRequestQuery is a wrapper
type NvmeSubsystemGetIterRequestQuery struct {
	XMLName              xml.Name               `xml:"query"`
	NvmeSubsystemInfoPtr *NvmeSubsystemInfoType `xml:"nvme-subsystem-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeSubsystemInfo is a 'getter' method
func (o *NvmeSubsystemGetIterRequestQuery) NvmeSubsystemInfo() NvmeSubsystemInfoType {
	var r NvmeSubsystemInfoType
	if o.NvmeSubsystemInfoPtr == nil {
		return r
	}
	r = *o.NvmeSubsystemInfoPtr
	return r
}

// SetNvmeSubsystemInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequestQuery) SetNvmeSubsystemInfo(newValue NvmeSubsystemInfoType) *NvmeSubsystemGetIterRequestQuery {
	o.NvmeSubsystemInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeSubsystemGetIterRequest) Query() NvmeSubsystemGetIterRequestQuery {
	var r NvmeSubsystemGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequest) SetQuery(newValue NvmeSubsystemGetIterRequestQuery) *NvmeSubsystemGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeSubsystemGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterRequest) SetTag(newValue string) *NvmeSubsystemGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeSubsystemGetIterResponseResultAttributesList is a wrapper
type NvmeSubsystemGetIterResponseResultAttributesList struct {
	XMLName              xml.Name                `xml:"attributes-list"`
	NvmeSubsystemInfoPtr []NvmeSubsystemInfoType `xml:"nvme-subsystem-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeSubsystemInfo is a 'getter' method
func (o *NvmeSubsystemGetIterResponseResultAttributesList) NvmeSubsystemInfo() []NvmeSubsystemInfoType {
	r := o.NvmeSubsystemInfoPtr
	return r
}

// SetNvmeSubsystemInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterResponseResultAttributesList) SetNvmeSubsystemInfo(newValue []NvmeSubsystemInfoType) *NvmeSubsystemGetIterResponseResultAttributesList {
	newSlice := make([]NvmeSubsystemInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeSubsystemInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeSubsystemGetIterResponseResultAttributesList) values() []NvmeSubsystemInfoType {
	r := o.NvmeSubsystemInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterResponseResultAttributesList) setValues(newValue []NvmeSubsystemInfoType) *NvmeSubsystemGetIterResponseResultAttributesList {
	newSlice := make([]NvmeSubsystemInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeSubsystemInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeSubsystemGetIterResponseResult) AttributesList() NvmeSubsystemGetIterResponseResultAttributesList {
	var r NvmeSubsystemGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterResponseResult) SetAttributesList(newValue NvmeSubsystemGetIterResponseResultAttributesList) *NvmeSubsystemGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeSubsystemGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterResponseResult) SetNextTag(newValue string) *NvmeSubsystemGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeSubsystemGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemGetIterResponseResult) SetNumRecords(newValue int) *NvmeSubsystemGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
