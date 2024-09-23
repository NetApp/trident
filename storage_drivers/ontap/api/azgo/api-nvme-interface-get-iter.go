// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeInterfaceGetIterRequest is a structure to represent a nvme-interface-get-iter Request ZAPI object
type NvmeInterfaceGetIterRequest struct {
	XMLName              xml.Name                                      `xml:"nvme-interface-get-iter"`
	DesiredAttributesPtr *NvmeInterfaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                          `xml:"max-records"`
	QueryPtr             *NvmeInterfaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                       `xml:"tag"`
}

// NvmeInterfaceGetIterResponse is a structure to represent a nvme-interface-get-iter Response ZAPI object
type NvmeInterfaceGetIterResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          NvmeInterfaceGetIterResponseResult `xml:"results"`
}

// NewNvmeInterfaceGetIterResponse is a factory method for creating new instances of NvmeInterfaceGetIterResponse objects
func NewNvmeInterfaceGetIterResponse() *NvmeInterfaceGetIterResponse {
	return &NvmeInterfaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeInterfaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeInterfaceGetIterResponseResult is a structure to represent a nvme-interface-get-iter Response Result ZAPI object
type NvmeInterfaceGetIterResponseResult struct {
	XMLName           xml.Name                                          `xml:"results"`
	ResultStatusAttr  string                                            `xml:"status,attr"`
	ResultReasonAttr  string                                            `xml:"reason,attr"`
	ResultErrnoAttr   string                                            `xml:"errno,attr"`
	AttributesListPtr *NvmeInterfaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                           `xml:"next-tag"`
	NumRecordsPtr     *int                                              `xml:"num-records"`
}

// NewNvmeInterfaceGetIterRequest is a factory method for creating new instances of NvmeInterfaceGetIterRequest objects
func NewNvmeInterfaceGetIterRequest() *NvmeInterfaceGetIterRequest {
	return &NvmeInterfaceGetIterRequest{}
}

// NewNvmeInterfaceGetIterResponseResult is a factory method for creating new instances of NvmeInterfaceGetIterResponseResult objects
func NewNvmeInterfaceGetIterResponseResult() *NvmeInterfaceGetIterResponseResult {
	return &NvmeInterfaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeInterfaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeInterfaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeInterfaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeInterfaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeInterfaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeInterfaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeInterfaceGetIterRequest", NewNvmeInterfaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeInterfaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeInterfaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeInterfaceGetIterResponse, error) {
	combined := NewNvmeInterfaceGetIterResponse()
	combined.Result.SetAttributesList(NvmeInterfaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeInterfaceGetIterResponseResultAttributesList{})
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

// NvmeInterfaceGetIterRequestDesiredAttributes is a wrapper
type NvmeInterfaceGetIterRequestDesiredAttributes struct {
	XMLName              xml.Name               `xml:"desired-attributes"`
	NvmeInterfaceInfoPtr *NvmeInterfaceInfoType `xml:"nvme-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeInterfaceInfo is a 'getter' method
func (o *NvmeInterfaceGetIterRequestDesiredAttributes) NvmeInterfaceInfo() NvmeInterfaceInfoType {
	var r NvmeInterfaceInfoType
	if o.NvmeInterfaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeInterfaceInfoPtr
	return r
}

// SetNvmeInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequestDesiredAttributes) SetNvmeInterfaceInfo(newValue NvmeInterfaceInfoType) *NvmeInterfaceGetIterRequestDesiredAttributes {
	o.NvmeInterfaceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeInterfaceGetIterRequest) DesiredAttributes() NvmeInterfaceGetIterRequestDesiredAttributes {
	var r NvmeInterfaceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequest) SetDesiredAttributes(newValue NvmeInterfaceGetIterRequestDesiredAttributes) *NvmeInterfaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeInterfaceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequest) SetMaxRecords(newValue int) *NvmeInterfaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeInterfaceGetIterRequestQuery is a wrapper
type NvmeInterfaceGetIterRequestQuery struct {
	XMLName              xml.Name               `xml:"query"`
	NvmeInterfaceInfoPtr *NvmeInterfaceInfoType `xml:"nvme-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeInterfaceInfo is a 'getter' method
func (o *NvmeInterfaceGetIterRequestQuery) NvmeInterfaceInfo() NvmeInterfaceInfoType {
	var r NvmeInterfaceInfoType
	if o.NvmeInterfaceInfoPtr == nil {
		return r
	}
	r = *o.NvmeInterfaceInfoPtr
	return r
}

// SetNvmeInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequestQuery) SetNvmeInterfaceInfo(newValue NvmeInterfaceInfoType) *NvmeInterfaceGetIterRequestQuery {
	o.NvmeInterfaceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeInterfaceGetIterRequest) Query() NvmeInterfaceGetIterRequestQuery {
	var r NvmeInterfaceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequest) SetQuery(newValue NvmeInterfaceGetIterRequestQuery) *NvmeInterfaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeInterfaceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterRequest) SetTag(newValue string) *NvmeInterfaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeInterfaceGetIterResponseResultAttributesList is a wrapper
type NvmeInterfaceGetIterResponseResultAttributesList struct {
	XMLName              xml.Name                `xml:"attributes-list"`
	NvmeInterfaceInfoPtr []NvmeInterfaceInfoType `xml:"nvme-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeInterfaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeInterfaceInfo is a 'getter' method
func (o *NvmeInterfaceGetIterResponseResultAttributesList) NvmeInterfaceInfo() []NvmeInterfaceInfoType {
	r := o.NvmeInterfaceInfoPtr
	return r
}

// SetNvmeInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterResponseResultAttributesList) SetNvmeInterfaceInfo(newValue []NvmeInterfaceInfoType) *NvmeInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]NvmeInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeInterfaceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeInterfaceGetIterResponseResultAttributesList) values() []NvmeInterfaceInfoType {
	r := o.NvmeInterfaceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterResponseResultAttributesList) setValues(newValue []NvmeInterfaceInfoType) *NvmeInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]NvmeInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeInterfaceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeInterfaceGetIterResponseResult) AttributesList() NvmeInterfaceGetIterResponseResultAttributesList {
	var r NvmeInterfaceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterResponseResult) SetAttributesList(newValue NvmeInterfaceGetIterResponseResultAttributesList) *NvmeInterfaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeInterfaceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterResponseResult) SetNextTag(newValue string) *NvmeInterfaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeInterfaceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeInterfaceGetIterResponseResult) SetNumRecords(newValue int) *NvmeInterfaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
