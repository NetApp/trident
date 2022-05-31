// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VserverShowAggrGetIterRequest is a structure to represent a vserver-show-aggr-get-iter Request ZAPI object
type VserverShowAggrGetIterRequest struct {
	XMLName              xml.Name                                        `xml:"vserver-show-aggr-get-iter"`
	DesiredAttributesPtr *VserverShowAggrGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                            `xml:"max-records"`
	QueryPtr             *VserverShowAggrGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                         `xml:"tag"`
	VserverPtr           *string                                         `xml:"vserver"`
}

// VserverShowAggrGetIterResponse is a structure to represent a vserver-show-aggr-get-iter Response ZAPI object
type VserverShowAggrGetIterResponse struct {
	XMLName         xml.Name                             `xml:"netapp"`
	ResponseVersion string                               `xml:"version,attr"`
	ResponseXmlns   string                               `xml:"xmlns,attr"`
	Result          VserverShowAggrGetIterResponseResult `xml:"results"`
}

// NewVserverShowAggrGetIterResponse is a factory method for creating new instances of VserverShowAggrGetIterResponse objects
func NewVserverShowAggrGetIterResponse() *VserverShowAggrGetIterResponse {
	return &VserverShowAggrGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VserverShowAggrGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VserverShowAggrGetIterResponseResult is a structure to represent a vserver-show-aggr-get-iter Response Result ZAPI object
type VserverShowAggrGetIterResponseResult struct {
	XMLName           xml.Name                                            `xml:"results"`
	ResultStatusAttr  string                                              `xml:"status,attr"`
	ResultReasonAttr  string                                              `xml:"reason,attr"`
	ResultErrnoAttr   string                                              `xml:"errno,attr"`
	AttributesListPtr *VserverShowAggrGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                             `xml:"next-tag"`
	NumRecordsPtr     *int                                                `xml:"num-records"`
}

// NewVserverShowAggrGetIterRequest is a factory method for creating new instances of VserverShowAggrGetIterRequest objects
func NewVserverShowAggrGetIterRequest() *VserverShowAggrGetIterRequest {
	return &VserverShowAggrGetIterRequest{}
}

// NewVserverShowAggrGetIterResponseResult is a factory method for creating new instances of VserverShowAggrGetIterResponseResult objects
func NewVserverShowAggrGetIterResponseResult() *VserverShowAggrGetIterResponseResult {
	return &VserverShowAggrGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VserverShowAggrGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VserverShowAggrGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverShowAggrGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*VserverShowAggrGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverShowAggrGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VserverShowAggrGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VserverShowAggrGetIterRequest", NewVserverShowAggrGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VserverShowAggrGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverShowAggrGetIterRequest) executeWithIteration(zr *ZapiRunner) (*VserverShowAggrGetIterResponse, error) {
	combined := NewVserverShowAggrGetIterResponse()
	combined.Result.SetAttributesList(VserverShowAggrGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(VserverShowAggrGetIterResponseResultAttributesList{})
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

// VserverShowAggrGetIterRequestDesiredAttributes is a wrapper
type VserverShowAggrGetIterRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	ShowAggregatesPtr *ShowAggregatesType `xml:"show-aggregates"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// ShowAggregates is a 'getter' method
func (o *VserverShowAggrGetIterRequestDesiredAttributes) ShowAggregates() ShowAggregatesType {
	var r ShowAggregatesType
	if o.ShowAggregatesPtr == nil {
		return r
	}
	r = *o.ShowAggregatesPtr
	return r
}

// SetShowAggregates is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequestDesiredAttributes) SetShowAggregates(newValue ShowAggregatesType) *VserverShowAggrGetIterRequestDesiredAttributes {
	o.ShowAggregatesPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VserverShowAggrGetIterRequest) DesiredAttributes() VserverShowAggrGetIterRequestDesiredAttributes {
	var r VserverShowAggrGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetDesiredAttributes(newValue VserverShowAggrGetIterRequestDesiredAttributes) *VserverShowAggrGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VserverShowAggrGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetMaxRecords(newValue int) *VserverShowAggrGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VserverShowAggrGetIterRequestQuery is a wrapper
type VserverShowAggrGetIterRequestQuery struct {
	XMLName           xml.Name            `xml:"query"`
	ShowAggregatesPtr *ShowAggregatesType `xml:"show-aggregates"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// ShowAggregates is a 'getter' method
func (o *VserverShowAggrGetIterRequestQuery) ShowAggregates() ShowAggregatesType {
	var r ShowAggregatesType
	if o.ShowAggregatesPtr == nil {
		return r
	}
	r = *o.ShowAggregatesPtr
	return r
}

// SetShowAggregates is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequestQuery) SetShowAggregates(newValue ShowAggregatesType) *VserverShowAggrGetIterRequestQuery {
	o.ShowAggregatesPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VserverShowAggrGetIterRequest) Query() VserverShowAggrGetIterRequestQuery {
	var r VserverShowAggrGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetQuery(newValue VserverShowAggrGetIterRequestQuery) *VserverShowAggrGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VserverShowAggrGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetTag(newValue string) *VserverShowAggrGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *VserverShowAggrGetIterRequest) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterRequest) SetVserver(newValue string) *VserverShowAggrGetIterRequest {
	o.VserverPtr = &newValue
	return o
}

// VserverShowAggrGetIterResponseResultAttributesList is a wrapper
type VserverShowAggrGetIterResponseResultAttributesList struct {
	XMLName           xml.Name             `xml:"attributes-list"`
	ShowAggregatesPtr []ShowAggregatesType `xml:"show-aggregates"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverShowAggrGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// ShowAggregates is a 'getter' method
func (o *VserverShowAggrGetIterResponseResultAttributesList) ShowAggregates() []ShowAggregatesType {
	r := o.ShowAggregatesPtr
	return r
}

// SetShowAggregates is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResultAttributesList) SetShowAggregates(newValue []ShowAggregatesType) *VserverShowAggrGetIterResponseResultAttributesList {
	newSlice := make([]ShowAggregatesType, len(newValue))
	copy(newSlice, newValue)
	o.ShowAggregatesPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VserverShowAggrGetIterResponseResultAttributesList) values() []ShowAggregatesType {
	r := o.ShowAggregatesPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResultAttributesList) setValues(newValue []ShowAggregatesType) *VserverShowAggrGetIterResponseResultAttributesList {
	newSlice := make([]ShowAggregatesType, len(newValue))
	copy(newSlice, newValue)
	o.ShowAggregatesPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *VserverShowAggrGetIterResponseResult) AttributesList() VserverShowAggrGetIterResponseResultAttributesList {
	var r VserverShowAggrGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetAttributesList(newValue VserverShowAggrGetIterResponseResultAttributesList) *VserverShowAggrGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VserverShowAggrGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetNextTag(newValue string) *VserverShowAggrGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *VserverShowAggrGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VserverShowAggrGetIterResponseResult) SetNumRecords(newValue int) *VserverShowAggrGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
