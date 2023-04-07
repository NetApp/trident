// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeGetIterRequest is a structure to represent a nvme-get-iter Request ZAPI object
type NvmeGetIterRequest struct {
	XMLName              xml.Name                             `xml:"nvme-get-iter"`
	DesiredAttributesPtr *NvmeGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                 `xml:"max-records"`
	QueryPtr             *NvmeGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                              `xml:"tag"`
}

// NvmeGetIterResponse is a structure to represent a nvme-get-iter Response ZAPI object
type NvmeGetIterResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          NvmeGetIterResponseResult `xml:"results"`
}

// NewNvmeGetIterResponse is a factory method for creating new instances of NvmeGetIterResponse objects
func NewNvmeGetIterResponse() *NvmeGetIterResponse {
	return &NvmeGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeGetIterResponseResult is a structure to represent a nvme-get-iter Response Result ZAPI object
type NvmeGetIterResponseResult struct {
	XMLName           xml.Name                                 `xml:"results"`
	ResultStatusAttr  string                                   `xml:"status,attr"`
	ResultReasonAttr  string                                   `xml:"reason,attr"`
	ResultErrnoAttr   string                                   `xml:"errno,attr"`
	AttributesListPtr *NvmeGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                  `xml:"next-tag"`
	NumRecordsPtr     *int                                     `xml:"num-records"`
}

// NewNvmeGetIterRequest is a factory method for creating new instances of NvmeGetIterRequest objects
func NewNvmeGetIterRequest() *NvmeGetIterRequest {
	return &NvmeGetIterRequest{}
}

// NewNvmeGetIterResponseResult is a factory method for creating new instances of NvmeGetIterResponseResult objects
func NewNvmeGetIterResponseResult() *NvmeGetIterResponseResult {
	return &NvmeGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeGetIterRequest", NewNvmeGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeGetIterResponse, error) {
	combined := NewNvmeGetIterResponse()
	combined.Result.SetAttributesList(NvmeGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeGetIterResponseResultAttributesList{})
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

// NvmeGetIterRequestDesiredAttributes is a wrapper
type NvmeGetIterRequestDesiredAttributes struct {
	XMLName                  xml.Name                   `xml:"desired-attributes"`
	NvmeTargetServiceInfoPtr *NvmeTargetServiceInfoType `xml:"nvme-target-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetServiceInfo is a 'getter' method
func (o *NvmeGetIterRequestDesiredAttributes) NvmeTargetServiceInfo() NvmeTargetServiceInfoType {
	var r NvmeTargetServiceInfoType
	if o.NvmeTargetServiceInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetServiceInfoPtr
	return r
}

// SetNvmeTargetServiceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequestDesiredAttributes) SetNvmeTargetServiceInfo(newValue NvmeTargetServiceInfoType) *NvmeGetIterRequestDesiredAttributes {
	o.NvmeTargetServiceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeGetIterRequest) DesiredAttributes() NvmeGetIterRequestDesiredAttributes {
	var r NvmeGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequest) SetDesiredAttributes(newValue NvmeGetIterRequestDesiredAttributes) *NvmeGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequest) SetMaxRecords(newValue int) *NvmeGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeGetIterRequestQuery is a wrapper
type NvmeGetIterRequestQuery struct {
	XMLName                  xml.Name                   `xml:"query"`
	NvmeTargetServiceInfoPtr *NvmeTargetServiceInfoType `xml:"nvme-target-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetServiceInfo is a 'getter' method
func (o *NvmeGetIterRequestQuery) NvmeTargetServiceInfo() NvmeTargetServiceInfoType {
	var r NvmeTargetServiceInfoType
	if o.NvmeTargetServiceInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetServiceInfoPtr
	return r
}

// SetNvmeTargetServiceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequestQuery) SetNvmeTargetServiceInfo(newValue NvmeTargetServiceInfoType) *NvmeGetIterRequestQuery {
	o.NvmeTargetServiceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeGetIterRequest) Query() NvmeGetIterRequestQuery {
	var r NvmeGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequest) SetQuery(newValue NvmeGetIterRequestQuery) *NvmeGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterRequest) SetTag(newValue string) *NvmeGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeGetIterResponseResultAttributesList is a wrapper
type NvmeGetIterResponseResultAttributesList struct {
	XMLName                  xml.Name                    `xml:"attributes-list"`
	NvmeTargetServiceInfoPtr []NvmeTargetServiceInfoType `xml:"nvme-target-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetServiceInfo is a 'getter' method
func (o *NvmeGetIterResponseResultAttributesList) NvmeTargetServiceInfo() []NvmeTargetServiceInfoType {
	r := o.NvmeTargetServiceInfoPtr
	return r
}

// SetNvmeTargetServiceInfo is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterResponseResultAttributesList) SetNvmeTargetServiceInfo(newValue []NvmeTargetServiceInfoType) *NvmeGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetServiceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeGetIterResponseResultAttributesList) values() []NvmeTargetServiceInfoType {
	r := o.NvmeTargetServiceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterResponseResultAttributesList) setValues(newValue []NvmeTargetServiceInfoType) *NvmeGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetServiceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeGetIterResponseResult) AttributesList() NvmeGetIterResponseResultAttributesList {
	var r NvmeGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterResponseResult) SetAttributesList(newValue NvmeGetIterResponseResultAttributesList) *NvmeGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterResponseResult) SetNextTag(newValue string) *NvmeGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeGetIterResponseResult) SetNumRecords(newValue int) *NvmeGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
