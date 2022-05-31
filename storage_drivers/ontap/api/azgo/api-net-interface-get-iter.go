// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NetInterfaceGetIterRequest is a structure to represent a net-interface-get-iter Request ZAPI object
type NetInterfaceGetIterRequest struct {
	XMLName              xml.Name                                     `xml:"net-interface-get-iter"`
	DesiredAttributesPtr *NetInterfaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                         `xml:"max-records"`
	QueryPtr             *NetInterfaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                      `xml:"tag"`
}

// NetInterfaceGetIterResponse is a structure to represent a net-interface-get-iter Response ZAPI object
type NetInterfaceGetIterResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NetInterfaceGetIterResponseResult `xml:"results"`
}

// NewNetInterfaceGetIterResponse is a factory method for creating new instances of NetInterfaceGetIterResponse objects
func NewNetInterfaceGetIterResponse() *NetInterfaceGetIterResponse {
	return &NetInterfaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NetInterfaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NetInterfaceGetIterResponseResult is a structure to represent a net-interface-get-iter Response Result ZAPI object
type NetInterfaceGetIterResponseResult struct {
	XMLName           xml.Name                                         `xml:"results"`
	ResultStatusAttr  string                                           `xml:"status,attr"`
	ResultReasonAttr  string                                           `xml:"reason,attr"`
	ResultErrnoAttr   string                                           `xml:"errno,attr"`
	AttributesListPtr *NetInterfaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                          `xml:"next-tag"`
	NumRecordsPtr     *int                                             `xml:"num-records"`
}

// NewNetInterfaceGetIterRequest is a factory method for creating new instances of NetInterfaceGetIterRequest objects
func NewNetInterfaceGetIterRequest() *NetInterfaceGetIterRequest {
	return &NetInterfaceGetIterRequest{}
}

// NewNetInterfaceGetIterResponseResult is a factory method for creating new instances of NetInterfaceGetIterResponseResult objects
func NewNetInterfaceGetIterResponseResult() *NetInterfaceGetIterResponseResult {
	return &NetInterfaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NetInterfaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NetInterfaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NetInterfaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NetInterfaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NetInterfaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NetInterfaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NetInterfaceGetIterRequest", NewNetInterfaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NetInterfaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NetInterfaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NetInterfaceGetIterResponse, error) {
	combined := NewNetInterfaceGetIterResponse()
	combined.Result.SetAttributesList(NetInterfaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NetInterfaceGetIterResponseResultAttributesList{})
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

// NetInterfaceGetIterRequestDesiredAttributes is a wrapper
type NetInterfaceGetIterRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	NetInterfaceInfoPtr *NetInterfaceInfoType `xml:"net-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NetInterfaceInfo is a 'getter' method
func (o *NetInterfaceGetIterRequestDesiredAttributes) NetInterfaceInfo() NetInterfaceInfoType {
	var r NetInterfaceInfoType
	if o.NetInterfaceInfoPtr == nil {
		return r
	}
	r = *o.NetInterfaceInfoPtr
	return r
}

// SetNetInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequestDesiredAttributes) SetNetInterfaceInfo(newValue NetInterfaceInfoType) *NetInterfaceGetIterRequestDesiredAttributes {
	o.NetInterfaceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NetInterfaceGetIterRequest) DesiredAttributes() NetInterfaceGetIterRequestDesiredAttributes {
	var r NetInterfaceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequest) SetDesiredAttributes(newValue NetInterfaceGetIterRequestDesiredAttributes) *NetInterfaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NetInterfaceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequest) SetMaxRecords(newValue int) *NetInterfaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NetInterfaceGetIterRequestQuery is a wrapper
type NetInterfaceGetIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	NetInterfaceInfoPtr *NetInterfaceInfoType `xml:"net-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NetInterfaceInfo is a 'getter' method
func (o *NetInterfaceGetIterRequestQuery) NetInterfaceInfo() NetInterfaceInfoType {
	var r NetInterfaceInfoType
	if o.NetInterfaceInfoPtr == nil {
		return r
	}
	r = *o.NetInterfaceInfoPtr
	return r
}

// SetNetInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequestQuery) SetNetInterfaceInfo(newValue NetInterfaceInfoType) *NetInterfaceGetIterRequestQuery {
	o.NetInterfaceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NetInterfaceGetIterRequest) Query() NetInterfaceGetIterRequestQuery {
	var r NetInterfaceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequest) SetQuery(newValue NetInterfaceGetIterRequestQuery) *NetInterfaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NetInterfaceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterRequest) SetTag(newValue string) *NetInterfaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NetInterfaceGetIterResponseResultAttributesList is a wrapper
type NetInterfaceGetIterResponseResultAttributesList struct {
	XMLName             xml.Name               `xml:"attributes-list"`
	NetInterfaceInfoPtr []NetInterfaceInfoType `xml:"net-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NetInterfaceInfo is a 'getter' method
func (o *NetInterfaceGetIterResponseResultAttributesList) NetInterfaceInfo() []NetInterfaceInfoType {
	r := o.NetInterfaceInfoPtr
	return r
}

// SetNetInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterResponseResultAttributesList) SetNetInterfaceInfo(newValue []NetInterfaceInfoType) *NetInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]NetInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NetInterfaceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NetInterfaceGetIterResponseResultAttributesList) values() []NetInterfaceInfoType {
	r := o.NetInterfaceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterResponseResultAttributesList) setValues(newValue []NetInterfaceInfoType) *NetInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]NetInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NetInterfaceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NetInterfaceGetIterResponseResult) AttributesList() NetInterfaceGetIterResponseResultAttributesList {
	var r NetInterfaceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterResponseResult) SetAttributesList(newValue NetInterfaceGetIterResponseResultAttributesList) *NetInterfaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NetInterfaceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterResponseResult) SetNextTag(newValue string) *NetInterfaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NetInterfaceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NetInterfaceGetIterResponseResult) SetNumRecords(newValue int) *NetInterfaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
