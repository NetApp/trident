// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeSubsystemMapGetIterRequest is a structure to represent a nvme-subsystem-map-get-iter Request ZAPI object
type NvmeSubsystemMapGetIterRequest struct {
	XMLName              xml.Name                                         `xml:"nvme-subsystem-map-get-iter"`
	DesiredAttributesPtr *NvmeSubsystemMapGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                             `xml:"max-records"`
	QueryPtr             *NvmeSubsystemMapGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                          `xml:"tag"`
}

// NvmeSubsystemMapGetIterResponse is a structure to represent a nvme-subsystem-map-get-iter Response ZAPI object
type NvmeSubsystemMapGetIterResponse struct {
	XMLName         xml.Name                              `xml:"netapp"`
	ResponseVersion string                                `xml:"version,attr"`
	ResponseXmlns   string                                `xml:"xmlns,attr"`
	Result          NvmeSubsystemMapGetIterResponseResult `xml:"results"`
}

// NewNvmeSubsystemMapGetIterResponse is a factory method for creating new instances of NvmeSubsystemMapGetIterResponse objects
func NewNvmeSubsystemMapGetIterResponse() *NvmeSubsystemMapGetIterResponse {
	return &NvmeSubsystemMapGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemMapGetIterResponseResult is a structure to represent a nvme-subsystem-map-get-iter Response Result ZAPI object
type NvmeSubsystemMapGetIterResponseResult struct {
	XMLName           xml.Name                                             `xml:"results"`
	ResultStatusAttr  string                                               `xml:"status,attr"`
	ResultReasonAttr  string                                               `xml:"reason,attr"`
	ResultErrnoAttr   string                                               `xml:"errno,attr"`
	AttributesListPtr *NvmeSubsystemMapGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                              `xml:"next-tag"`
	NumRecordsPtr     *int                                                 `xml:"num-records"`
}

// NewNvmeSubsystemMapGetIterRequest is a factory method for creating new instances of NvmeSubsystemMapGetIterRequest objects
func NewNvmeSubsystemMapGetIterRequest() *NvmeSubsystemMapGetIterRequest {
	return &NvmeSubsystemMapGetIterRequest{}
}

// NewNvmeSubsystemMapGetIterResponseResult is a factory method for creating new instances of NvmeSubsystemMapGetIterResponseResult objects
func NewNvmeSubsystemMapGetIterResponseResult() *NvmeSubsystemMapGetIterResponseResult {
	return &NvmeSubsystemMapGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemMapGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemMapGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemMapGetIterRequest", NewNvmeSubsystemMapGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemMapGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeSubsystemMapGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeSubsystemMapGetIterResponse, error) {
	combined := NewNvmeSubsystemMapGetIterResponse()
	combined.Result.SetAttributesList(NvmeSubsystemMapGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeSubsystemMapGetIterResponseResultAttributesList{})
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

// NvmeSubsystemMapGetIterRequestDesiredAttributes is a wrapper
type NvmeSubsystemMapGetIterRequestDesiredAttributes struct {
	XMLName                       xml.Name                        `xml:"desired-attributes"`
	NvmeTargetSubsystemMapInfoPtr *NvmeTargetSubsystemMapInfoType `xml:"nvme-target-subsystem-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemMapInfo is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequestDesiredAttributes) NvmeTargetSubsystemMapInfo() NvmeTargetSubsystemMapInfoType {
	var r NvmeTargetSubsystemMapInfoType
	if o.NvmeTargetSubsystemMapInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemMapInfoPtr
	return r
}

// SetNvmeTargetSubsystemMapInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequestDesiredAttributes) SetNvmeTargetSubsystemMapInfo(newValue NvmeTargetSubsystemMapInfoType) *NvmeSubsystemMapGetIterRequestDesiredAttributes {
	o.NvmeTargetSubsystemMapInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequest) DesiredAttributes() NvmeSubsystemMapGetIterRequestDesiredAttributes {
	var r NvmeSubsystemMapGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequest) SetDesiredAttributes(newValue NvmeSubsystemMapGetIterRequestDesiredAttributes) *NvmeSubsystemMapGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequest) SetMaxRecords(newValue int) *NvmeSubsystemMapGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeSubsystemMapGetIterRequestQuery is a wrapper
type NvmeSubsystemMapGetIterRequestQuery struct {
	XMLName                       xml.Name                        `xml:"query"`
	NvmeTargetSubsystemMapInfoPtr *NvmeTargetSubsystemMapInfoType `xml:"nvme-target-subsystem-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemMapInfo is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequestQuery) NvmeTargetSubsystemMapInfo() NvmeTargetSubsystemMapInfoType {
	var r NvmeTargetSubsystemMapInfoType
	if o.NvmeTargetSubsystemMapInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemMapInfoPtr
	return r
}

// SetNvmeTargetSubsystemMapInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequestQuery) SetNvmeTargetSubsystemMapInfo(newValue NvmeTargetSubsystemMapInfoType) *NvmeSubsystemMapGetIterRequestQuery {
	o.NvmeTargetSubsystemMapInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequest) Query() NvmeSubsystemMapGetIterRequestQuery {
	var r NvmeSubsystemMapGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequest) SetQuery(newValue NvmeSubsystemMapGetIterRequestQuery) *NvmeSubsystemMapGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeSubsystemMapGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterRequest) SetTag(newValue string) *NvmeSubsystemMapGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeSubsystemMapGetIterResponseResultAttributesList is a wrapper
type NvmeSubsystemMapGetIterResponseResultAttributesList struct {
	XMLName                       xml.Name                         `xml:"attributes-list"`
	NvmeTargetSubsystemMapInfoPtr []NvmeTargetSubsystemMapInfoType `xml:"nvme-target-subsystem-map-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemMapInfo is a 'getter' method
func (o *NvmeSubsystemMapGetIterResponseResultAttributesList) NvmeTargetSubsystemMapInfo() []NvmeTargetSubsystemMapInfoType {
	r := o.NvmeTargetSubsystemMapInfoPtr
	return r
}

// SetNvmeTargetSubsystemMapInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterResponseResultAttributesList) SetNvmeTargetSubsystemMapInfo(newValue []NvmeTargetSubsystemMapInfoType) *NvmeSubsystemMapGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemMapInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemMapInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeSubsystemMapGetIterResponseResultAttributesList) values() []NvmeTargetSubsystemMapInfoType {
	r := o.NvmeTargetSubsystemMapInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterResponseResultAttributesList) setValues(newValue []NvmeTargetSubsystemMapInfoType) *NvmeSubsystemMapGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemMapInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemMapInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeSubsystemMapGetIterResponseResult) AttributesList() NvmeSubsystemMapGetIterResponseResultAttributesList {
	var r NvmeSubsystemMapGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterResponseResult) SetAttributesList(newValue NvmeSubsystemMapGetIterResponseResultAttributesList) *NvmeSubsystemMapGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeSubsystemMapGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterResponseResult) SetNextTag(newValue string) *NvmeSubsystemMapGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeSubsystemMapGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapGetIterResponseResult) SetNumRecords(newValue int) *NvmeSubsystemMapGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
